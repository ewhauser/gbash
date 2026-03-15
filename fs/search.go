package fs

import (
	"context"
	"errors"
)

// ErrSearchUnsupported reports that a filesystem or provider does not expose
// search capability.
//
// Experimental: This API is experimental and subject to change.
var ErrSearchUnsupported = errors.New("fs: search unsupported")

// SearchCapable reports whether a filesystem can expose a search provider for a
// particular virtual path.
//
// Experimental: This API is experimental and subject to change.
type SearchCapable interface {
	// SearchProviderForPath returns the search provider responsible for name.
	//
	// Providers are mount- or filesystem-scoped. Callers should resolve a
	// provider for each root they want to search instead of assuming one global
	// index covers the entire namespace.
	//
	// Experimental: This API is experimental and subject to change.
	SearchProviderForPath(name string) (SearchProvider, bool)
}

// SearchProvider executes search queries against an optional filesystem-local
// index or search backend.
//
// Experimental: This API is experimental and subject to change.
type SearchProvider interface {
	// Search executes query against the provider's scope.
	//
	// Experimental: This API is experimental and subject to change.
	Search(ctx context.Context, query *SearchQuery) (SearchResult, error)

	// SearchCapabilities reports which query and result features the provider
	// can satisfy directly.
	//
	// Experimental: This API is experimental and subject to change.
	SearchCapabilities() SearchCapabilities

	// IndexStatus reports the provider's current generation and freshness.
	//
	// Experimental: This API is experimental and subject to change.
	IndexStatus() IndexStatus
}

// SearchIndexer extends SearchProvider with mutation hooks for providers that
// can stay synchronized with filesystem changes.
//
// Experimental: This API is experimental and subject to change.
type SearchIndexer interface {
	SearchProvider

	// ApplySearchMutation updates the provider after a filesystem mutation.
	//
	// Experimental: This API is experimental and subject to change.
	ApplySearchMutation(ctx context.Context, mutation *SearchMutation) error
}

// SearchCapabilities describes the supported search surface for a provider.
//
// Experimental: This API is experimental and subject to change.
type SearchCapabilities struct {
	LiteralSearch           bool
	IgnoreCaseLiteralSearch bool
	RootRestriction         bool
	IncludeGlobs            bool
	ExcludeGlobs            bool
	Offsets                 bool
	ApproximateResults      bool
	VerifiedResults         bool
	GenerationTracking      bool
}

// SearchQuery describes a filesystem-scoped full-text lookup.
//
// Experimental: This API is experimental and subject to change.
type SearchQuery struct {
	Root         string
	Literal      string
	IgnoreCase   bool
	IncludeGlobs []string
	ExcludeGlobs []string
	Limit        int
	WantOffsets  bool
}

// SearchHit describes one matching file candidate.
//
// Experimental: This API is experimental and subject to change.
type SearchHit struct {
	Path        string
	Offsets     []int64
	Approximate bool
	Verified    bool
	Stale       bool
	Truncated   bool
}

// SearchResult is the complete response to a search query.
//
// Experimental: This API is experimental and subject to change.
type SearchResult struct {
	Hits      []SearchHit
	Status    IndexStatus
	Truncated bool
}

// IndexStatus reports provider freshness and backend details.
//
// Experimental: This API is experimental and subject to change.
type IndexStatus struct {
	CurrentGeneration uint64
	IndexedGeneration uint64
	Backend           string
	Synchronous       bool
}

// SearchMutationKind describes the filesystem change applied to an indexer.
//
// Experimental: This API is experimental and subject to change.
type SearchMutationKind string

const (
	// SearchMutationWrite records the full contents of a file path.
	//
	// Experimental: This API is experimental and subject to change.
	SearchMutationWrite SearchMutationKind = "write"

	// SearchMutationRemove removes a file path or subtree from the index.
	//
	// Experimental: This API is experimental and subject to change.
	SearchMutationRemove SearchMutationKind = "remove"

	// SearchMutationRename rewrites a file path or subtree prefix in the index.
	//
	// Experimental: This API is experimental and subject to change.
	SearchMutationRename SearchMutationKind = "rename"

	// SearchMutationMetadata advances provider generation without updating file
	// contents.
	//
	// Experimental: This API is experimental and subject to change.
	SearchMutationMetadata SearchMutationKind = "metadata"
)

// SearchMutation describes one filesystem update applied to a SearchIndexer.
//
// Experimental: This API is experimental and subject to change.
type SearchMutation struct {
	Kind    SearchMutationKind
	Path    string
	OldPath string
	NewPath string
	Data    []byte
}

// NewInMemorySearchProvider returns the built-in synchronous in-memory search
// provider.
//
// Experimental: This API is experimental and subject to change.
func NewInMemorySearchProvider() SearchIndexer {
	return newMemorySearchProvider()
}

// NewUnsupportedSearchProvider returns a provider that always reports search as
// unsupported.
//
// Experimental: This API is experimental and subject to change.
func NewUnsupportedSearchProvider() SearchProvider {
	return unsupportedSearchProvider{}
}
