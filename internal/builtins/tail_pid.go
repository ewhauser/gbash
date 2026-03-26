package builtins

import (
	"errors"

	pubhost "github.com/ewhauser/gbash/host"
)

var errTailPIDUnsupported = errors.New("tail pid liveness unsupported")

func tailPIDIsAlive(pid int) (bool, error) {
	alive, err := pubhost.ProcessExists(pid)
	if errors.Is(err, pubhost.ErrProcessExistenceUnsupported) {
		return false, errTailPIDUnsupported
	}
	return alive, err
}
