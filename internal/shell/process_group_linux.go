//go:build linux

package shell

import "golang.org/x/sys/unix"

func currentProcessGroup() int {
	return unix.Getpgrp()
}
