//go:build windows

package commands

import "os"

func tailPIDIsAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, FindProcess always succeeds. Release the handle.
	_ = p.Release()
	return true
}
