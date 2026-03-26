package builtins

import pubhost "github.com/ewhauser/gbash/host"

func tailPIDIsAlive(pid int) (bool, error) {
	return pubhost.ProcessExists(pid)
}
