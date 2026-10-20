package fs

import (
	"context"
	"errors"
	"io"
	stdfs "io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// OverlayFS is a copy-on-write filesystem with a writable in-memory upper
// layer and a read-only lower layer.
type OverlayFS struct {
	lower FileSystem
	upper *MemoryFS

	mu         sync.RWMutex
	cwd        string
	tombstones map[string]struct{}
	// lowerDirs caches lower-layer directories seen while walking path
	// prefixes. The lower layer is never mutated through the overlay, so
	// this spares backends such as HostFS from re-resolving every prefix of
	// every path. It assumes the lower directory structure stays stable for
	// the overlay's lifetime; a live lower such as HostFS can break that if
	// the host replaces a directory, so entries are only trusted for the type
	// of a path prefix. Metadata copied up into the upper layer is always
	// re-read from the lower layer.
	lowerDirs map[string]stdfs.FileInfo
}

// NewOverlay creates a concrete overlay filesystem over lower.
func NewOverlay(lower FileSystem) *OverlayFS {
	if lower == nil {
		lower = NewMemory()
	}
	return &OverlayFS{
		lower:      lower,
		upper:      newMemoryWithImplicitParents(),
		cwd:        "/",
		tombstones: make(map[string]struct{}),
		lowerDirs:  make(map[string]stdfs.FileInfo),
	}
}

func (o *OverlayFS) Open(ctx context.Context, name string) (File, error) {
	return o.OpenFile(ctx, name, os.O_RDONLY, 0)
}

func (o *OverlayFS) OpenFile(ctx context.Context, name string, flag int, perm stdfs.FileMode) (File, error) {
	abs := o.resolve(name)
	if !hasWriteIntent(flag) {
		resolved, _, source, err := o.resolveMerged(ctx, abs, true)
		if err != nil {
			return nil, toPathError("open", abs, err)
		}
		if source == overlaySourceUpper {
			return o.upper.OpenFile(ctx, resolved, flag, perm)
		}
		return o.lower.OpenFile(ctx, resolved, flag, perm)
	}

	missing := overlayMissingNone
	if flag&os.O_CREATE != 0 {
		missing = overlayMissingFinal
	}
	// O_CREATE|O_EXCL never follows a final symlink: an existing link, even a
	// dangling one, makes the open fail.
	exclusive := flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL
	resolved, info, source, err := o.resolveMergedPath(ctx, abs, !exclusive, missing)
	if err != nil {
		if flag&os.O_CREATE != 0 {
			err = createParentError(err)
		}
		return nil, toPathError("open", abs, err)
	}
	if exclusive && info != nil {
		return nil, &os.PathError{Op: "open", Path: abs, Err: stdfs.ErrExist}
	}
	switch {
	case info == nil:
		if flag&os.O_CREATE == 0 {
			return nil, &os.PathError{Op: "open", Path: abs, Err: stdfs.ErrNotExist}
		}
		if err := o.copyUpAncestors(ctx, resolved); err != nil {
			return nil, toPathError("open", abs, err)
		}
	case source == overlaySourceLower:
		if info.IsDir() {
			return nil, &os.PathError{Op: "open", Path: abs, Err: stdfs.ErrInvalid}
		}
		// Copy up the resolved target, never a symlink that led to it.
		if err := o.copyUpNode(ctx, resolved, info); err != nil {
			return nil, toPathError("open", abs, err)
		}
	}
	return o.upper.OpenFile(ctx, resolved, flag, perm)
}

func (o *OverlayFS) Stat(ctx context.Context, name string) (stdfs.FileInfo, error) {
	abs := o.resolve(name)
	_, info, _, err := o.resolveMerged(ctx, abs, true)
	if err != nil {
		return nil, toPathError("stat", abs, err)
	}
	return info, nil
}

func (o *OverlayFS) Lstat(ctx context.Context, name string) (stdfs.FileInfo, error) {
	abs := o.resolve(name)
	_, info, _, err := o.resolveMerged(ctx, abs, false)
	if err != nil {
		return nil, toPathError("lstat", abs, err)
	}
	return info, nil
}

func (o *OverlayFS) ReadDir(ctx context.Context, name string) ([]stdfs.DirEntry, error) {
	requested := o.resolve(name)
	abs, _, _, err := o.resolveMerged(ctx, requested, true)
	if err != nil {
		return nil, toPathError("readdir", requested, err)
	}
	upperInfo, upperExists, err := lstatMaybe(ctx, o.upper, abs)
	if err != nil {
		return nil, toPathError("readdir", abs, err)
	}
	if upperExists && !upperInfo.IsDir() {
		return nil, &os.PathError{Op: "readdir", Path: abs, Err: stdfs.ErrInvalid}
	}
	if !upperExists && o.isHidden(abs) {
		return nil, &os.PathError{Op: "readdir", Path: abs, Err: stdfs.ErrNotExist}
	}

	lowerInfo, lowerExists, err := lstatMaybe(ctx, o.lower, abs)
	if err != nil {
		return nil, toPathError("readdir", abs, err)
	}
	if lowerExists && !lowerInfo.IsDir() && !upperExists { //nolint:nilaway // lowerInfo is non-nil when lowerExists is true; nilaway cannot track the boolean correlation
		return nil, &os.PathError{Op: "readdir", Path: abs, Err: stdfs.ErrInvalid}
	}
	if !upperExists && !lowerExists {
		return nil, &os.PathError{Op: "readdir", Path: abs, Err: stdfs.ErrNotExist}
	}

	entriesByName := make(map[string]stdfs.DirEntry)
	if upperExists {
		upperEntries, err := o.upper.ReadDir(ctx, abs)
		if err != nil {
			return nil, err
		}
		for _, entry := range upperEntries {
			entriesByName[entry.Name()] = entry
		}
	}

	if lowerExists && !o.isExactTombstone(abs) {
		lowerEntries, err := o.lower.ReadDir(ctx, abs)
		if err != nil {
			return nil, err
		}
		for _, entry := range lowerEntries {
			child := path.Join(abs, entry.Name())
			if o.isHidden(child) {
				continue
			}
			if _, exists := entriesByName[entry.Name()]; exists {
				continue
			}
			entriesByName[entry.Name()] = entry
		}
	}

	names := make([]string, 0, len(entriesByName))
	for name := range entriesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]stdfs.DirEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, entriesByName[name])
	}
	return entries, nil
}

func (o *OverlayFS) Readlink(ctx context.Context, name string) (string, error) {
	abs := o.resolve(name)
	resolved, _, source, err := o.resolveMerged(ctx, abs, false)
	if err != nil {
		return "", toPathError("readlink", abs, err)
	}
	target, err := o.readlinkAt(ctx, source, resolved)
	if err != nil {
		return "", toPathError("readlink", abs, err)
	}
	return target, nil
}

func (o *OverlayFS) Realpath(ctx context.Context, name string) (string, error) {
	abs := o.resolve(name)
	resolved, _, _, err := o.resolveMerged(ctx, abs, true)
	if err != nil {
		return "", &os.PathError{Op: "realpath", Path: abs, Err: unwrapPathError(err)}
	}
	return resolved, nil
}

func (o *OverlayFS) Symlink(ctx context.Context, target, linkName string) error {
	requested := o.resolve(linkName)
	abs, info, _, err := o.resolveMergedPath(ctx, requested, false, overlayMissingFinal)
	if err != nil {
		return toPathError("symlink", requested, createParentError(err))
	}
	if info != nil {
		return &os.PathError{Op: "symlink", Path: abs, Err: stdfs.ErrExist}
	}
	if err := o.copyUpAncestors(ctx, abs); err != nil {
		return toPathError("symlink", abs, err)
	}
	if err := o.upper.Symlink(ctx, target, abs); err != nil {
		return toPathError("symlink", abs, err)
	}
	o.clearTombstonesUnder(abs)
	return nil
}

func (o *OverlayFS) Link(ctx context.Context, oldName, newName string) error {
	oldAbs, info, source, err := o.resolveMerged(ctx, o.resolve(oldName), false)
	if err != nil {
		return &os.PathError{Op: "link", Path: o.resolve(oldName), Err: stdfs.ErrNotExist}
	}
	if info.IsDir() {
		return &os.PathError{Op: "link", Path: oldAbs, Err: stdfs.ErrInvalid}
	}
	newAbs, newInfo, _, err := o.resolveMergedPath(ctx, o.resolve(newName), false, overlayMissingFinal)
	if err != nil {
		return toPathError("link", o.resolve(newName), createParentError(err))
	}
	if newInfo != nil {
		return &os.PathError{Op: "link", Path: newAbs, Err: stdfs.ErrExist}
	}
	if err := o.copyUpAncestors(ctx, newAbs); err != nil {
		return toPathError("link", newAbs, err)
	}

	switch source {
	case overlaySourceUpper:
		if err := o.upper.Link(ctx, oldAbs, newAbs); err != nil {
			return toPathError("link", oldAbs, err)
		}
	case overlaySourceLower:
		if _, err := o.upper.Lstat(ctx, oldAbs); errors.Is(err, stdfs.ErrNotExist) {
			if err := o.copyUpNode(ctx, oldAbs, info); err != nil {
				return toPathError("link", oldAbs, err)
			}
		} else if err != nil {
			return toPathError("link", oldAbs, err)
		}
		if err := o.upper.Link(ctx, oldAbs, newAbs); err != nil {
			return toPathError("link", oldAbs, err)
		}
	default:
		return &os.PathError{Op: "link", Path: oldAbs, Err: stdfs.ErrNotExist}
	}
	o.clearTombstonesUnder(newAbs)
	return nil
}

func (o *OverlayFS) Chmod(ctx context.Context, name string, mode stdfs.FileMode) error {
	requested := o.resolve(name)
	abs, info, source, err := o.resolveMerged(ctx, requested, true)
	if err != nil {
		return toPathError("chmod", requested, err)
	}
	if source == overlaySourceLower {
		if err := o.copyUpNode(ctx, abs, info); err != nil {
			return toPathError("chmod", abs, err)
		}
	}
	if err := o.upper.Chmod(ctx, abs, mode); err != nil {
		return toPathError("chmod", abs, err)
	}
	return nil
}

func (o *OverlayFS) Chown(ctx context.Context, name string, uid, gid uint32, follow bool) error {
	requested := o.resolve(name)
	abs, info, source, err := o.resolveMerged(ctx, requested, follow)
	if err != nil {
		return toPathError("chown", requested, err)
	}
	if source == overlaySourceLower {
		if err := o.copyUpNode(ctx, abs, info); err != nil {
			return toPathError("chown", abs, err)
		}
	}
	if err := o.upper.Chown(ctx, abs, uid, gid, follow); err != nil {
		return toPathError("chown", abs, err)
	}
	return nil
}

func (o *OverlayFS) Chtimes(ctx context.Context, name string, atime, mtime time.Time) error {
	requested := o.resolve(name)
	abs, info, source, err := o.resolveMerged(ctx, requested, true)
	if err != nil {
		return toPathError("chtimes", requested, err)
	}
	if source == overlaySourceLower {
		if err := o.copyUpNode(ctx, abs, info); err != nil {
			return toPathError("chtimes", abs, err)
		}
	}
	if err := o.upper.Chtimes(ctx, abs, atime, mtime); err != nil {
		return toPathError("chtimes", abs, err)
	}
	return nil
}

func (o *OverlayFS) Lchtimes(ctx context.Context, name string, atime, mtime time.Time) error {
	requested := o.resolve(name)
	abs, info, source, err := o.resolveMerged(ctx, requested, false)
	if err != nil {
		return toPathError("lchtimes", requested, err)
	}
	if source == overlaySourceLower {
		if err := o.copyUpNode(ctx, abs, info); err != nil {
			return toPathError("lchtimes", abs, err)
		}
	}
	if err := o.upper.Lchtimes(ctx, abs, atime, mtime); err != nil {
		return toPathError("lchtimes", abs, err)
	}
	return nil
}

func (o *OverlayFS) MkdirAll(ctx context.Context, name string, perm stdfs.FileMode) error {
	abs := o.resolve(name)
	if abs == "/" {
		return nil
	}
	current := "/"
	parts := strings.Split(strings.TrimPrefix(abs, "/"), "/")
	for i, part := range parts {
		// current is free of symlinks in the merged view, so only the new
		// component needs to be inspected (and followed if it is a link).
		next := joinChildPath(current, part)
		info, _, err := o.visiblePrefixInfo(ctx, next)
		if errors.Is(err, stdfs.ErrNotExist) {
			if err := o.copyUpAncestors(ctx, next); err != nil {
				return toPathError("mkdir", next, err)
			}
			return o.upper.MkdirAll(ctx, Resolve(next, path.Join(parts[i+1:]...)), perm)
		}
		if err != nil {
			return toPathError("mkdir", next, err)
		}
		if info.Mode()&stdfs.ModeSymlink != 0 {
			next, info, _, err = o.resolveMerged(ctx, next, true)
			if err != nil {
				return toPathError("mkdir", joinChildPath(current, part), err)
			}
		}
		if !info.IsDir() {
			return &os.PathError{Op: "mkdir", Path: joinChildPath(current, part), Err: stdfs.ErrInvalid}
		}
		current = next
	}
	return nil
}

func (o *OverlayFS) Mkfifo(ctx context.Context, name string, perm stdfs.FileMode) error {
	requested := o.resolve(name)
	abs, info, _, err := o.resolveMergedPath(ctx, requested, false, overlayMissingFinal)
	if err != nil {
		return toPathError("mkfifo", requested, createParentError(err))
	}
	if info != nil {
		return &os.PathError{Op: "mkfifo", Path: abs, Err: stdfs.ErrExist}
	}
	if err := o.copyUpAncestors(ctx, abs); err != nil {
		return toPathError("mkfifo", abs, err)
	}
	if err := o.upper.Mkfifo(ctx, abs, perm); err != nil {
		return toPathError("mkfifo", abs, err)
	}
	o.clearTombstonesUnder(abs)
	return nil
}

func (o *OverlayFS) Remove(ctx context.Context, name string, recursive bool) error {
	requested := o.resolve(name)
	if requested == "/" {
		return &os.PathError{Op: "remove", Path: requested, Err: stdfs.ErrPermission}
	}

	abs, info, source, err := o.resolveMerged(ctx, requested, false)
	if err != nil {
		return &os.PathError{Op: "remove", Path: requested, Err: stdfs.ErrNotExist}
	}
	if abs == "/" {
		return &os.PathError{Op: "remove", Path: requested, Err: stdfs.ErrPermission}
	}
	if info.IsDir() && !recursive {
		entries, err := o.ReadDir(ctx, abs)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return &os.PathError{Op: "remove", Path: abs, Err: stdfs.ErrInvalid}
		}
	}

	if source == overlaySourceUpper {
		if err := removeMaybe(ctx, o.upper, abs, recursive); err != nil {
			return toPathError("remove", abs, err)
		}
	}
	if _, lowerExists, err := lstatMaybe(ctx, o.lower, abs); err != nil {
		return toPathError("remove", abs, err)
	} else if lowerExists {
		o.addTombstone(abs)
	}
	return nil
}

func (o *OverlayFS) Rename(ctx context.Context, oldName, newName string) error {
	oldRequested := o.resolve(oldName)
	newRequested := o.resolve(newName)
	if oldRequested == "/" || newRequested == "/" {
		return &os.PathError{Op: "rename", Path: oldRequested, Err: stdfs.ErrPermission}
	}
	oldAbs, info, source, err := o.resolveMerged(ctx, oldRequested, false)
	if err != nil {
		return &os.PathError{Op: "rename", Path: oldRequested, Err: stdfs.ErrNotExist}
	}
	newAbs, newInfo, _, err := o.resolveMergedPath(ctx, newRequested, false, overlayMissingFinal)
	if err != nil {
		return toPathError("rename", newRequested, createParentError(err))
	}
	if oldAbs == "/" || newAbs == "/" {
		return &os.PathError{Op: "rename", Path: oldRequested, Err: stdfs.ErrPermission}
	}
	if oldAbs == newAbs {
		return nil
	}
	if newInfo != nil {
		return &os.PathError{Op: "rename", Path: newAbs, Err: stdfs.ErrExist}
	}
	if info.IsDir() && isDescendantPath(newAbs, oldAbs) {
		return &os.PathError{Op: "rename", Path: newAbs, Err: stdfs.ErrInvalid}
	}

	if err := o.copyUpAncestors(ctx, newAbs); err != nil {
		return toPathError("rename", newAbs, err)
	}
	if err := o.moveMerged(ctx, oldAbs, info, source, newAbs); err != nil {
		return toPathError("rename", oldAbs, err)
	}
	// Upper entries were moved rather than copied, so oldAbs may already be
	// gone from the merged view.
	if err := o.Remove(ctx, oldAbs, true); err != nil && !errors.Is(err, stdfs.ErrNotExist) {
		return err
	}
	return nil
}

// moveMerged recreates the merged entry at src under dst in the upper layer.
// Upper non-directories are moved as nodes, keeping their content and hard
// links. Lower files are copied up lazily, so their content is not read until
// it is opened. Directories are recreated with their metadata and their merged
// children are moved recursively.
func (o *OverlayFS) moveMerged(ctx context.Context, src string, info stdfs.FileInfo, source overlaySource, dst string) error {
	if !info.IsDir() {
		if source == overlaySourceUpper {
			return o.upper.Rename(ctx, src, dst)
		}
		return o.copyUpLowerEntry(ctx, src, info, dst)
	}
	if err := cloneDirNode(ctx, o.upper, dst, info); err != nil {
		return err
	}
	entries, err := o.ReadDir(ctx, src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		childSrc := joinChildPath(src, entry.Name())
		childInfo, childSource, err := o.visibleInfo(ctx, childSrc)
		if err != nil {
			return err
		}
		if err := o.moveMerged(ctx, childSrc, childInfo, childSource, joinChildPath(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (o *OverlayFS) Getwd() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.cwd
}

func (o *OverlayFS) Chdir(name string) error {
	abs := o.resolve(name)
	_, info, _, err := o.resolveMerged(context.Background(), abs, true)
	if err != nil {
		return &os.PathError{Op: "chdir", Path: abs, Err: stdfs.ErrNotExist}
	}
	if !info.IsDir() {
		return &os.PathError{Op: "chdir", Path: abs, Err: stdfs.ErrInvalid}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cwd = abs
	return nil
}

type overlaySource uint8

const (
	overlaySourceNone overlaySource = iota
	overlaySourceUpper
	overlaySourceLower
)

// copyUpNode copies the node at abs, which the merged view currently takes
// from the lower layer, into the upper layer so a write, link, or metadata
// change can be recorded there. Missing upper ancestors are created with their
// lower metadata. A directory is copied as a bare node: its children stay in the
// lower layer, where the merged ReadDir still finds them and tombstones keep
// deleted ones hidden.
func (o *OverlayFS) copyUpNode(ctx context.Context, abs string, info stdfs.FileInfo) error {
	if err := o.copyUpAncestors(ctx, abs); err != nil {
		return err
	}
	if info.IsDir() {
		return cloneDirNode(ctx, o.upper, abs, info)
	}
	return o.copyUpLowerEntry(ctx, abs, info, abs)
}

// copyUpAncestors creates every ancestor of abs that the merged view takes
// from the lower layer in the upper layer, carrying its lower mode, ownership,
// and modification time. It runs before anything is written under abs so the
// upper layer never shadows a lower directory with default metadata. abs must
// already be resolved against the merged view.
func (o *OverlayFS) copyUpAncestors(ctx context.Context, abs string) error {
	parent := parentDir(abs)
	if parent == "/" {
		return nil
	}
	current := "/"
	for part := range strings.SplitSeq(strings.TrimPrefix(parent, "/"), "/") {
		current = joinChildPath(current, part)
		_, source, err := o.visiblePrefixInfo(ctx, current)
		if err != nil {
			return err
		}
		if source == overlaySourceUpper {
			continue
		}
		// The prefix cache only vouches for the type, so re-read the metadata.
		dirInfo, err := o.lower.Lstat(ctx, current)
		if err != nil {
			return err
		}
		if err := cloneDirNode(ctx, o.upper, current, dirInfo); err != nil {
			return err
		}
	}
	return nil
}

// copyUpLowerEntry copies the lower non-directory at src to dst in the upper
// layer. Regular files become lazy nodes that read the lower content only
// when opened, so a file the lower layer refuses to read (for example one
// above HostFS's read cap) can still be truncated, renamed, or have its
// metadata changed.
func (o *OverlayFS) copyUpLowerEntry(ctx context.Context, src string, info stdfs.FileInfo, dst string) error {
	if !info.Mode().IsRegular() {
		return clonePathWithInfo(ctx, o.lower, src, info, o.upper, dst)
	}
	lower := o.lower
	return cloneLazyFile(o.upper, Clean(dst), info, func(ctx context.Context) ([]byte, error) {
		file, err := lower.Open(ctx, src)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		return io.ReadAll(file)
	})
}

func (o *OverlayFS) visibleInfo(ctx context.Context, abs string) (stdfs.FileInfo, overlaySource, error) {
	if info, err := o.upper.Lstat(ctx, abs); err == nil {
		return info, overlaySourceUpper, nil
	} else if !errors.Is(err, stdfs.ErrNotExist) {
		return nil, overlaySourceNone, err
	}
	if o.isHidden(abs) {
		return nil, overlaySourceNone, stdfs.ErrNotExist
	}
	info, err := o.lower.Lstat(ctx, abs)
	if err != nil {
		return nil, overlaySourceNone, err
	}
	return info, overlaySourceLower, nil
}

// visiblePrefixInfo is visibleInfo for a directory prefix of a longer path:
// lower-layer directory results are cached because callers only inspect their
// type.
func (o *OverlayFS) visiblePrefixInfo(ctx context.Context, abs string) (stdfs.FileInfo, overlaySource, error) {
	if info, err := o.upper.Lstat(ctx, abs); err == nil {
		return info, overlaySourceUpper, nil
	} else if !errors.Is(err, stdfs.ErrNotExist) {
		return nil, overlaySourceNone, err
	}
	if o.isHidden(abs) {
		return nil, overlaySourceNone, stdfs.ErrNotExist
	}
	o.mu.RLock()
	info, ok := o.lowerDirs[abs]
	o.mu.RUnlock()
	if ok {
		return info, overlaySourceLower, nil
	}
	info, err := o.lower.Lstat(ctx, abs)
	if err != nil {
		return nil, overlaySourceNone, err
	}
	if info.IsDir() {
		o.mu.Lock()
		o.lowerDirs[abs] = info
		o.mu.Unlock()
	}
	return info, overlaySourceLower, nil
}

func (o *OverlayFS) visibleComponentInfo(ctx context.Context, abs string, isLast bool) (stdfs.FileInfo, overlaySource, error) {
	if isLast {
		return o.visibleInfo(ctx, abs)
	}
	return o.visiblePrefixInfo(ctx, abs)
}

// overlayMissing controls how resolveMergedPath treats path components that do
// not exist in the merged view.
type overlayMissing uint8

const (
	// overlayMissingNone reports ErrNotExist for any missing component.
	overlayMissingNone overlayMissing = iota
	// overlayMissingFinal accepts a missing final component. Creating
	// operations use it so a missing parent fails with ENOENT instead of being
	// materialized in the upper layer.
	overlayMissingFinal
)

// resolveMergedPath resolves abs against the merged view of both layers. Symlinks
// in intermediate components are always followed and the final component is
// followed when followFinal is set. Each link is read from the layer that
// currently provides it and its target is resolved through the merged view, so
// a link in one layer may point at an entry that only exists in the other.
//
// The returned path contains no symlinks other than an unfollowed final
// component. When the path is missing and the missing mode allows it, the
// resolved path is returned with a nil info and overlaySourceNone.
func (o *OverlayFS) resolveMergedPath(ctx context.Context, abs string, followFinal bool, missing overlayMissing) (string, stdfs.FileInfo, overlaySource, error) {
	return o.resolveMergedDepth(ctx, abs, followFinal, missing, 0)
}

// resolveMerged is resolveMergedPath for a path that must exist.
func (o *OverlayFS) resolveMerged(ctx context.Context, abs string, followFinal bool) (string, stdfs.FileInfo, overlaySource, error) {
	resolved, info, source, err := o.resolveMergedDepth(ctx, abs, followFinal, overlayMissingNone, 0)
	if err != nil {
		return "", nil, overlaySourceNone, err
	}
	if info == nil {
		return "", nil, overlaySourceNone, stdfs.ErrNotExist
	}
	return resolved, info, source, nil
}

func (o *OverlayFS) resolveMergedDepth(ctx context.Context, abs string, followFinal bool, missing overlayMissing, depth int) (string, stdfs.FileInfo, overlaySource, error) {
	abs = Clean(abs)
	if depth > maxSymlinkDepth {
		return "", nil, overlaySourceNone, errTooManySymlinks
	}
	if abs == "/" {
		info, source, err := o.visibleInfo(ctx, abs)
		if err != nil {
			return "", nil, overlaySourceNone, err
		}
		return abs, info, source, nil
	}

	current := "/"
	parts := strings.Split(strings.TrimPrefix(abs, "/"), "/")
	for i, part := range parts {
		next := joinChildPath(current, part)
		isLast := i == len(parts)-1
		info, source, err := o.visibleComponentInfo(ctx, next, isLast)
		if err != nil {
			if !errors.Is(err, stdfs.ErrNotExist) {
				return "", nil, overlaySourceNone, err
			}
			if isLast && missing == overlayMissingFinal {
				return next, nil, overlaySourceNone, nil
			}
			return "", nil, overlaySourceNone, stdfs.ErrNotExist
		}
		if info.Mode()&stdfs.ModeSymlink != 0 && (!isLast || followFinal) {
			target, err := o.readlinkAt(ctx, source, next)
			if err != nil {
				return "", nil, overlaySourceNone, unwrapPathError(err)
			}
			resolved := Resolve(current, target)
			if !isLast {
				resolved = Resolve(resolved, path.Join(parts[i+1:]...))
			}
			return o.resolveMergedDepth(ctx, resolved, followFinal, missing, depth+1)
		}
		if isLast {
			return next, info, source, nil
		}
		if !info.IsDir() {
			return "", nil, overlaySourceNone, stdfs.ErrInvalid
		}
		current = next
	}
	return abs, nil, overlaySourceNone, stdfs.ErrNotExist
}

func (o *OverlayFS) readlinkAt(ctx context.Context, source overlaySource, abs string) (string, error) {
	switch source {
	case overlaySourceUpper:
		return o.upper.Readlink(ctx, abs)
	case overlaySourceLower:
		return o.lower.Readlink(ctx, abs)
	default:
		return "", stdfs.ErrNotExist
	}
}

func (o *OverlayFS) resolve(name string) string {
	return Resolve(o.Getwd(), name)
}

func (o *OverlayFS) isHidden(name string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	for current := Clean(name); ; current = parentDir(current) {
		if _, ok := o.tombstones[current]; ok {
			return true
		}
		if current == "/" {
			return false
		}
	}
}

func (o *OverlayFS) isExactTombstone(name string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	_, ok := o.tombstones[Clean(name)]
	return ok
}

func (o *OverlayFS) addTombstone(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tombstones[Clean(name)] = struct{}{}
}

func (o *OverlayFS) clearTombstonesUnder(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	name = Clean(name)
	for candidate := range o.tombstones {
		if candidate == name || strings.HasPrefix(candidate, name+"/") {
			delete(o.tombstones, candidate)
		}
	}
}

func lstatMaybe(ctx context.Context, fsys FileSystem, name string) (stdfs.FileInfo, bool, error) {
	info, err := fsys.Lstat(ctx, name)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return info, true, nil
}

func removeMaybe(ctx context.Context, fsys FileSystem, name string, recursive bool) error {
	err := fsys.Remove(ctx, name, recursive)
	if err != nil && errors.Is(err, stdfs.ErrNotExist) {
		return nil
	}
	return err
}

func toPathError(op, name string, err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr
	}
	return &os.PathError{Op: op, Path: name, Err: err}
}

func unwrapPathError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

func isDescendantPath(name, parent string) bool {
	parent = Clean(parent)
	name = Clean(name)
	return name != parent && strings.HasPrefix(name, parent+"/")
}

var _ FileSystem = (*OverlayFS)(nil)
var _ FIFOFileSystem = (*OverlayFS)(nil)
