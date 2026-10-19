package policy

import (
	"context"
	"errors"
	stdfs "io/fs"
	"path"

	gbfs "github.com/ewhauser/gbash/fs"
)

// maxSymlinkHops bounds symlink resolution while checking paths that may not
// exist yet.
const maxSymlinkHops = 40

var errTooManySymlinks = errors.New("too many levels of symbolic links")

func CheckPath(ctx context.Context, pol Policy, fsys gbfs.FileSystem, action FileAction, target string) error {
	target = cleanAbs(target)
	if pol == nil {
		return nil
	}
	if err := pol.AllowPath(ctx, action, target); err != nil {
		return err
	}
	if fsys == nil {
		return nil
	}

	resolved, traversed, err := resolveTarget(ctx, fsys, action, target)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) || errors.Is(err, stdfs.ErrInvalid) {
			return nil
		}
		return err
	}
	if traversed && pol.SymlinkMode() == SymlinkDeny {
		return &DeniedError{
			Subject: string(action) + " " + `"` + target + `"`,
			Reason:  "symlink traversal denied",
		}
	}
	if resolved != target {
		if err := pol.AllowPath(ctx, action, resolved); err != nil {
			return err
		}
	}
	return nil
}

func resolveTarget(ctx context.Context, fsys gbfs.FileSystem, action FileAction, target string) (resolved string, traversed bool, err error) {
	switch action {
	case FileActionRead, FileActionStat, FileActionReadDir:
		resolved, err := fsys.Realpath(ctx, target)
		return cleanAbs(resolved), cleanAbs(resolved) != cleanAbs(target), err
	case FileActionLstat, FileActionReadlink, FileActionRemove, FileActionRename:
		// These operate on the final entry itself, but backends still follow
		// symlinks in every parent component.
		resolved, err := resolveMaybeMissing(ctx, fsys, target, false, 0)
		if err != nil {
			return "", false, err
		}
		return resolved, resolved != cleanAbs(target), nil
	case FileActionWrite, FileActionMkdir:
		// Creating operations may materialize missing components, so resolve
		// through the deepest existing ancestor and any dangling symlinks.
		resolved, err := resolveMaybeMissing(ctx, fsys, target, true, 0)
		if err != nil {
			return "", false, err
		}
		return resolved, resolved != cleanAbs(target), nil
	default:
		return target, false, nil
	}
}

// resolveMaybeMissing resolves target the way a backend would when creating
// or mutating it, tolerating missing components. Existing paths are resolved
// with Realpath so backend namespaces are honored; a dangling symlink is
// followed from its resolved parent directory because Realpath cannot resolve
// it. When followFinal is false the final component is left unresolved.
func resolveMaybeMissing(ctx context.Context, fsys gbfs.FileSystem, target string, followFinal bool, hops int) (string, error) {
	target = cleanAbs(target)
	if hops > maxSymlinkHops {
		return "", errTooManySymlinks
	}
	if followFinal {
		resolved, err := fsys.Realpath(ctx, target)
		if err == nil {
			return cleanAbs(resolved), nil
		}
		if !errors.Is(err, stdfs.ErrNotExist) {
			return "", err
		}
	}
	if target == "/" {
		return target, nil
	}

	parent, err := resolveMaybeMissing(ctx, fsys, path.Dir(target), true, hops)
	if err != nil {
		return "", err
	}
	candidate := cleanAbs(path.Join(parent, path.Base(target)))
	if !followFinal {
		return candidate, nil
	}

	info, err := fsys.Lstat(ctx, candidate)
	if err != nil {
		if errors.Is(err, stdfs.ErrNotExist) {
			return candidate, nil
		}
		return "", err
	}
	if info.Mode()&stdfs.ModeSymlink == 0 {
		return candidate, nil
	}
	linkTarget, err := fsys.Readlink(ctx, candidate)
	if err != nil {
		return "", err
	}
	next := linkTarget
	if !path.IsAbs(next) {
		next = path.Join(parent, next)
	}
	return resolveMaybeMissing(ctx, fsys, next, true, hops+1)
}
