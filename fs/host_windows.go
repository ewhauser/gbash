//go:build windows

package fs

import "errors"

var errHostUnsupported = errors.New("host-backed filesystem is unsupported on Windows")

// HostFS is unavailable on Windows.
type HostFS struct{}

// NewHost returns an unsupported error on Windows.
func NewHost(HostOptions) (*HostFS, error) {
	return nil, errHostUnsupported
}
