//go:build !windows

package commands

import "syscall"

func tailPIDIsAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
