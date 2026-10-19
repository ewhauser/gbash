//go:build unix

package server

import (
	"os"
	"syscall"
	"testing"
)

// TestListenUnixSocketNeverExposesPermissiveSocket watches the socket path
// while listening under a permissive umask. The socket must already be 0600
// the first time it is visible, so no other user can connect before the chmod.
// It changes the process umask, so it must not run in parallel.
//
//nolint:paralleltest // mutates the process-wide umask
func TestListenUnixSocketNeverExposesPermissiveSocket(t *testing.T) {
	oldMask := syscall.Umask(0)
	defer syscall.Umask(oldMask)

	for range 50 {
		socket := newShortSocketPath(t)

		stop := make(chan struct{})
		seen := make(chan os.FileMode, 1)
		go func() {
			for {
				select {
				case <-stop:
					close(seen)
					return
				default:
				}
				if info, err := os.Lstat(socket); err == nil {
					seen <- info.Mode().Perm()
					close(seen)
					return
				}
			}
		}()

		ln, err := listenUnixSocket(t.Context(), socket)
		close(stop)
		if err != nil {
			t.Fatalf("listenUnixSocket(%q) error = %v", socket, err)
		}
		_ = ln.Close()

		if mode, ok := <-seen; ok && mode != 0o600 {
			t.Fatalf("socket first observed with mode %o, want %o", mode, 0o600)
		}
	}
}
