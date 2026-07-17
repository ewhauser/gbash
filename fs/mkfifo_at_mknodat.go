//go:build aix || dragonfly

package fs

import (
	stdfs "io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func mkfifoAt(parent *os.File, base string, perm stdfs.FileMode) error {
	return unix.Mknodat(int(parent.Fd()), base, uint32(perm.Perm())|unix.S_IFIFO, 0)
}
