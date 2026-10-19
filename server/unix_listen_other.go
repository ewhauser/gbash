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
func listenPrivateUnixSocket(ctx context.Context, socketPath string) (net.Listener, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("server: listen on unix socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("server: chmod socket: %w", err)
	}
	return ln, nil
}
