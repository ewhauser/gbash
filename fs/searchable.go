package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdfs "io/fs"
	"os"
	"path"
	"strings"
	"time"
)

// NewSearchableFileSystem wraps fsys with the experimental filesystem search
// capability.
//
// When provider is nil, the built-in in-memory provider is used.
//
// Experimental: This API is experimental and subject to change.
func NewSearchableFileSystem(ctx context.Context, fsys FileSystem, provider SearchIndexer) (FileSystem, error) {
	if fsys == nil {
		return nil, stdfs.ErrInvalid
	}
	if provider == nil {
		provider = NewInMemorySearchProvider()
	}

	snapshot, err := buildSearchSnapshot(ctx, fsys)
	if err != nil {
		return nil, err
	}
	if loader, ok := provider.(snapshotSearchIndexer); ok {
		if err := loader.loadSnapshot(snapshot); err != nil {
			return nil, err
		}
	} else {
		for name, data := range snapshot {
			if err := provider.ApplySearchMutation(ctx, &SearchMutation{
				Kind: SearchMutationWrite,
				Path: name,
				Data: data,
			}); err != nil {
				return nil, err
			}
		}
	}

	return &searchableFS{
		inner:  fsys,
		search: provider,
	}, nil
}

// NewSearchableFactory wraps each filesystem created by base with the
// experimental filesystem search capability.
//
// When newProvider is nil, the built-in in-memory provider is used for each
// filesystem instance.
//
// Experimental: This API is experimental and subject to change.
func NewSearchableFactory(base Factory, newProvider func() SearchIndexer) Factory {
	if base == nil {
		base = Memory()
	}
	return FactoryFunc(func(ctx context.Context) (FileSystem, error) {
		fsys, err := base.New(ctx)
		if err != nil {
			return nil, err
		}
		var provider SearchIndexer
		if newProvider != nil {
			provider = newProvider()
		}
		return NewSearchableFileSystem(ctx, fsys, provider)
	})
}

type searchableFS struct {
	inner  FileSystem
	search SearchIndexer
}

func (s *searchableFS) SearchProviderForPath(string) (SearchProvider, bool) {
	if s == nil || s.search == nil {
		return nil, false
	}
	return s.search, true
}

func (s *searchableFS) Open(ctx context.Context, name string) (File, error) {
	return s.inner.Open(ctx, name)
}

func (s *searchableFS) OpenFile(ctx context.Context, name string, flag int, perm stdfs.FileMode) (File, error) {
	abs := Resolve(s.inner.Getwd(), name)
	existedBefore := false
	if flag&os.O_CREATE != 0 || flag&os.O_TRUNC != 0 {
		_, err := s.inner.Lstat(ctx, abs)
		switch {
		case err == nil:
			existedBefore = true
		case errors.Is(err, stdfs.ErrNotExist):
		default:
			return nil, err
		}
	}

	file, err := s.inner.OpenFile(ctx, abs, flag, perm)
	if err != nil {
		return nil, err
	}
	if !canWrite(flag) {
		return file, nil
	}

	return &searchableFile{
		file:       file,
		owner:      s,
		path:       abs,
		dirtyOnEnd: flag&os.O_TRUNC != 0 || (flag&os.O_CREATE != 0 && !existedBefore),
	}, nil
}

func (s *searchableFS) Stat(ctx context.Context, name string) (stdfs.FileInfo, error) {
	return s.inner.Stat(ctx, name)
}

func (s *searchableFS) Lstat(ctx context.Context, name string) (stdfs.FileInfo, error) {
	return s.inner.Lstat(ctx, name)
}

func (s *searchableFS) ReadDir(ctx context.Context, name string) ([]stdfs.DirEntry, error) {
	return s.inner.ReadDir(ctx, name)
}

func (s *searchableFS) Readlink(ctx context.Context, name string) (string, error) {
	return s.inner.Readlink(ctx, name)
}

func (s *searchableFS) Realpath(ctx context.Context, name string) (string, error) {
	return s.inner.Realpath(ctx, name)
}

func (s *searchableFS) Symlink(ctx context.Context, target, linkName string) error {
	if err := s.inner.Symlink(ctx, target, linkName); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind: SearchMutationMetadata,
		Path: Resolve(s.inner.Getwd(), linkName),
	})
}

func (s *searchableFS) Link(ctx context.Context, oldName, newName string) error {
	if err := s.inner.Link(ctx, oldName, newName); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind: SearchMutationMetadata,
		Path: Resolve(s.inner.Getwd(), newName),
	})
}

func (s *searchableFS) Chown(ctx context.Context, name string, uid, gid uint32, follow bool) error {
	if err := s.inner.Chown(ctx, name, uid, gid, follow); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind: SearchMutationMetadata,
		Path: Resolve(s.inner.Getwd(), name),
	})
}

func (s *searchableFS) Chmod(ctx context.Context, name string, mode stdfs.FileMode) error {
	if err := s.inner.Chmod(ctx, name, mode); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind: SearchMutationMetadata,
		Path: Resolve(s.inner.Getwd(), name),
	})
}

func (s *searchableFS) Chtimes(ctx context.Context, name string, atime, mtime time.Time) error {
	if err := s.inner.Chtimes(ctx, name, atime, mtime); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind: SearchMutationMetadata,
		Path: Resolve(s.inner.Getwd(), name),
	})
}

func (s *searchableFS) MkdirAll(ctx context.Context, name string, perm stdfs.FileMode) error {
	if err := s.inner.MkdirAll(ctx, name, perm); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind: SearchMutationMetadata,
		Path: Resolve(s.inner.Getwd(), name),
	})
}

func (s *searchableFS) Remove(ctx context.Context, name string, recursive bool) error {
	abs := Resolve(s.inner.Getwd(), name)
	if err := s.inner.Remove(ctx, abs, recursive); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind: SearchMutationRemove,
		Path: abs,
	})
}

func (s *searchableFS) Rename(ctx context.Context, oldName, newName string) error {
	oldAbs := Resolve(s.inner.Getwd(), oldName)
	newAbs := Resolve(s.inner.Getwd(), newName)
	if err := s.inner.Rename(ctx, oldAbs, newAbs); err != nil {
		return err
	}
	return s.search.ApplySearchMutation(ctx, &SearchMutation{
		Kind:    SearchMutationRename,
		OldPath: oldAbs,
		NewPath: newAbs,
	})
}

func (s *searchableFS) Getwd() string {
	return s.inner.Getwd()
}

func (s *searchableFS) Chdir(name string) error {
	return s.inner.Chdir(name)
}

type searchableFile struct {
	file       File
	owner      *searchableFS
	path       string
	dirty      bool
	dirtyOnEnd bool
}

func (f *searchableFile) Read(p []byte) (int, error) {
	return f.file.Read(p)
}

func (f *searchableFile) Write(p []byte) (int, error) {
	n, err := f.file.Write(p)
	if n > 0 {
		f.dirty = true
	}
	return n, err
}

func (f *searchableFile) Close() error {
	if err := f.file.Close(); err != nil {
		return err
	}
	if !f.dirty && !f.dirtyOnEnd {
		return nil
	}

	data, err := readAllSearchFile(context.Background(), f.owner.inner, f.path)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) {
			return f.owner.search.ApplySearchMutation(context.Background(), &SearchMutation{
				Kind: SearchMutationRemove,
				Path: f.path,
			})
		}
		return err
	}
	return f.owner.search.ApplySearchMutation(context.Background(), &SearchMutation{
		Kind: SearchMutationWrite,
		Path: f.path,
		Data: data,
	})
}

func (f *searchableFile) Stat() (stdfs.FileInfo, error) {
	return f.file.Stat()
}

func buildSearchSnapshot(ctx context.Context, fsys FileSystem) (map[string][]byte, error) {
	out := make(map[string][]byte)
	if err := walkSearchSnapshot(ctx, fsys, "/", out); err != nil {
		return nil, err
	}
	return out, nil
}

func walkSearchSnapshot(ctx context.Context, fsys FileSystem, current string, out map[string][]byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	linfo, err := fsys.Lstat(ctx, current)
	if err != nil {
		return err
	}
	if linfo.Mode()&stdfs.ModeSymlink != 0 {
		info, err := fsys.Stat(ctx, current)
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := readAllSearchFile(ctx, fsys, current)
		if err != nil {
			return err
		}
		out[Clean(current)] = data
		return nil
	}
	if !linfo.IsDir() {
		data, err := readAllSearchFile(ctx, fsys, current)
		if err != nil {
			return err
		}
		out[Clean(current)] = data
		return nil
	}

	entries, err := fsys.ReadDir(ctx, current)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := walkSearchSnapshot(ctx, fsys, joinChildPath(current, entry.Name()), out); err != nil {
			return err
		}
	}
	return nil
}

func readAllSearchFile(ctx context.Context, fsys FileSystem, name string) ([]byte, error) {
	file, err := fsys.Open(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

// SearchProviderForPath returns a translated provider for the mounted
// filesystem or base filesystem responsible for name.
//
// Experimental: This API is experimental and subject to change.
func (m *MountableFS) SearchProviderForPath(name string) (SearchProvider, bool) {
	abs := m.resolve(name)
	entry, rel, mounted, synthetic := m.route(abs)
	if synthetic {
		return nil, false
	}
	if mounted {
		capable, ok := entry.fs.(SearchCapable)
		if !ok {
			return nil, false
		}
		provider, ok := capable.SearchProviderForPath(rel)
		if !ok {
			return nil, false
		}
		return translatedSearchProvider{
			inner:      provider,
			mountPoint: entry.mountPoint,
		}, true
	}
	capable, ok := m.base.(SearchCapable)
	if !ok {
		return nil, false
	}
	return capable.SearchProviderForPath(abs)
}

type translatedSearchProvider struct {
	inner      SearchProvider
	mountPoint string
}

func (p translatedSearchProvider) Search(ctx context.Context, query *SearchQuery) (SearchResult, error) {
	if query == nil {
		return SearchResult{}, fmt.Errorf("fs: search query is required")
	}
	innerQuery := *query
	root, err := p.translateRoot(query.Root)
	if err != nil {
		return SearchResult{}, err
	}
	innerQuery.Root = root

	result, err := p.inner.Search(ctx, &innerQuery)
	if err != nil {
		return SearchResult{}, err
	}
	for i := range result.Hits {
		result.Hits[i].Path = p.translatePath(result.Hits[i].Path)
	}
	return result, nil
}

func (p translatedSearchProvider) SearchCapabilities() SearchCapabilities {
	return p.inner.SearchCapabilities()
}

func (p translatedSearchProvider) IndexStatus() IndexStatus {
	return p.inner.IndexStatus()
}

func (p translatedSearchProvider) translateRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "/", nil
	}
	root = Clean(root)
	if root == p.mountPoint {
		return "/", nil
	}
	if strings.HasPrefix(root, p.mountPoint+"/") {
		return Clean(strings.TrimPrefix(root, p.mountPoint)), nil
	}
	return "", ErrSearchUnsupported
}

func (p translatedSearchProvider) translatePath(innerPath string) string {
	innerPath = Clean(innerPath)
	if innerPath == "/" {
		return p.mountPoint
	}
	return Clean(path.Join(p.mountPoint, strings.TrimPrefix(innerPath, "/")))
}
