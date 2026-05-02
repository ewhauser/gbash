//go:build darwin

package fs

import (
	stdfs "io/fs"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func mkfifoAt(parent *os.File, base string, perm stdfs.FileMode) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := unix.PthreadFchdir(int(parent.Fd())); err != nil {
		return err
	}
	defer func() { _ = unix.PthreadFchdir(-1) }()

	return unix.Mkfifo(base, uint32(perm.Perm()))
}
