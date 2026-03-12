package commands

import (
	"context"
	"errors"
	stdfs "io/fs"
	"strings"

	"github.com/ewhauser/gbash/policy"
)

type RM struct{}

func NewRM() *RM {
	return &RM{}
}

func (c *RM) Name() string {
	return "rm"
}

func (c *RM) Run(ctx context.Context, inv *Invocation) error {
	args := inv.Args
	recursive := false
	force := false
	allowDir := false

	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		if strings.HasPrefix(args[0], "--") || !applyRMShortFlags(args[0], &recursive, &force, &allowDir) {
			return exitf(inv, 1, "rm: unsupported flag %s", args[0])
		}
		args = args[1:]
	}

	if len(args) == 0 {
		return exitf(inv, 1, "rm: missing operand")
	}

	for _, name := range args {
		abs, err := allowPath(ctx, inv, policy.FileActionRemove, name)
		if err != nil {
			return err
		}
		info, err := inv.FS.Lstat(ctx, abs)
		if err != nil {
			if force && errors.Is(err, stdfs.ErrNotExist) {
				continue
			}
			return &ExitError{Code: 1, Err: err}
		}
		if info.IsDir() && !recursive && !allowDir {
			return exitf(inv, 1, "rm: cannot remove '%s': Is a directory", name)
		}
		if err := inv.FS.Remove(ctx, abs, recursive); err != nil {
			if force && errors.Is(err, stdfs.ErrNotExist) {
				continue
			}
			return &ExitError{Code: 1, Err: err}
		}
	}

	return nil
}

func applyRMShortFlags(arg string, recursive, force, allowDir *bool) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	for _, ch := range arg[1:] {
		switch ch {
		case 'r', 'R':
			*recursive = true
		case 'f':
			*force = true
		case 'd':
			*allowDir = true
		default:
			return false
		}
	}
	return true
}

var _ Command = (*RM)(nil)
