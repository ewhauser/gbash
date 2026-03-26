package host

import "errors"

// ErrProcessExistenceUnsupported reports that the current host adapter does not
// support probing whether a PID is still alive.
var ErrProcessExistenceUnsupported = errors.New("process existence checks are unsupported on this platform")
