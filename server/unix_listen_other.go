//go:build !unix

package server

import (
	"context"
	"fmt"
	"net"
	"os"
)

// listenPrivateUnixSocket binds a Unix socket at socketPath. Platforms
// without Unix file modes rely on the socket directory's access controls.
func listenPrivateUnixSocket(ctx context.Context, socketPath string) (net.Listener, os.FileInfo, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "unix", socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("server: listen on unix socket %s: %w", socketPath, unwrapOpError(err))
	}
	if unixLn, ok := ln.(*net.UnixListener); ok {
		unixLn.SetUnlinkOnClose(false)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(socketPath)
		return nil, nil, fmt.Errorf("server: chmod socket %s: %w", socketPath, unwrapPathError(err))
	}
	info, err := os.Lstat(socketPath)
	if err != nil {
		_ = ln.Close()
		_ = os.Remove(socketPath)
		return nil, nil, fmt.Errorf("server: stat socket %s: %w", socketPath, unwrapPathError(err))
	}
	return ln, info, nil
}
