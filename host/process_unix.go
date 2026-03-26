//go:build unix

package host

import (
	"errors"
	"os"
	"syscall"
)

// ProcessExists reports whether pid currently refers to a live process.
func ProcessExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	err = proc.Signal(syscall.Signal(0))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrProcessDone):
		return false, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	case errors.Is(err, syscall.EPERM):
		return true, nil
	default:
		return false, err
	}
}
