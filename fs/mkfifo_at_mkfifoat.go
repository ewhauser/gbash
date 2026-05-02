//go:build linux || netbsd || openbsd || solaris || zos

package fs

import (
	stdfs "io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func mkfifoAt(parent *os.File, base string, perm stdfs.FileMode) error {
	return unix.Mkfifoat(int(parent.Fd()), base, uint32(perm.Perm()))
}
