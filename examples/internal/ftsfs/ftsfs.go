package ftsfs

import gbfs "github.com/ewhauser/gbash/fs"

// NewFactory returns the searchable/full-text example's filesystem shape.
//
// When base is nil, it defaults to an in-memory filesystem.
func NewFactory(base gbfs.Factory) gbfs.Factory {
	return gbfs.NewSearchableFactory(base, nil)
}
