//go:build unix

package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// listenPrivateUnixSocket binds a Unix socket at socketPath that no other
// local user can connect to at any point.
//
// The socket is first bound inside a freshly created 0700 directory next to
// socketPath, so it is unreachable by other users while its mode still
// reflects the process umask. Once chmodded to 0600 it is renamed into place.
// The listener stays bound to the same socket inode across the rename.
func listenPrivateUnixSocket(ctx context.Context, socketPath string) (net.Listener, error) {
	stagingDir, err := os.MkdirTemp(filepath.Dir(socketPath), ".gbs")
	if err != nil {
		return nil, fmt.Errorf("server: create socket staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()
	if err := os.Chmod(stagingDir, 0o700); err != nil {
		return nil, fmt.Errorf("server: chmod socket staging directory: %w", err)
	}

	stagingPath := filepath.Join(stagingDir, "s")
	ln, err := (&net.ListenConfig{}).Listen(ctx, "unix", stagingPath)
	if err != nil {
		return nil, fmt.Errorf("server: listen on unix socket: %w", err)
	}
	// The listener would otherwise unlink stagingPath on Close, which no
	// longer names this socket after the rename. Callers remove socketPath.
	if unixLn, ok := ln.(*net.UnixListener); ok {
		unixLn.SetUnlinkOnClose(false)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = ln.Close()
		}
	}()
	if err := os.Chmod(stagingPath, 0o600); err != nil {
		return nil, fmt.Errorf("server: chmod socket: %w", err)
	}
	if err := os.Rename(stagingPath, socketPath); err != nil {
		return nil, fmt.Errorf("server: move socket into place: %w", err)
	}
	cleanup = false
	return ln, nil
}
