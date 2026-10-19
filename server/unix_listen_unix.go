//go:build unix

package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// maxUnixSocketPathLen is the longest socket path the kernel accepts, leaving
// room for the trailing NUL in sockaddr_un.sun_path.
var maxUnixSocketPathLen = len(syscall.RawSockaddrUnix{}.Path) - 1

// listenPrivateUnixSocket binds a Unix socket at socketPath that no other
// local user can connect to at any point.
//
// The socket is first bound inside a freshly created 0700 directory next to
// socketPath, so it is unreachable by other users while its mode still
// reflects the process umask. Once chmodded to 0600 it is hard-linked into
// place, which fails instead of replacing a socket another server bound in
// the meantime. The listener stays bound to the same socket inode.
func listenPrivateUnixSocket(ctx context.Context, socketPath string) (net.Listener, os.FileInfo, error) {
	if len(socketPath) > maxUnixSocketPathLen {
		return nil, nil, fmt.Errorf("server: socket path is %d bytes, longer than the %d-byte limit: %s", len(socketPath), maxUnixSocketPathLen, socketPath)
	}
	stagingDir, err := makeSocketStagingDir(filepath.Dir(socketPath))
	if err != nil {
		return nil, nil, fmt.Errorf("server: create socket staging directory for %s: %w", socketPath, err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	stagingPath := filepath.Join(stagingDir, "s")
	if len(stagingPath) > maxUnixSocketPathLen {
		return nil, nil, fmt.Errorf("server: socket path %s is too close to the %d-byte limit to stage safely; use a path at most %d bytes long", socketPath, maxUnixSocketPathLen, maxUnixSocketPathLen-(len(stagingPath)-len(socketPath)))
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "unix", stagingPath)
	if err != nil {
		return nil, nil, fmt.Errorf("server: listen on unix socket %s: %w", socketPath, unwrapOpError(err))
	}
	// The listener would otherwise unlink stagingPath on Close, which no
	// longer names this socket once it is in place. The returned listener
	// removes socketPath itself, and only while it still names this socket.
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
		return nil, nil, fmt.Errorf("server: chmod socket %s: %w", socketPath, unwrapPathError(err))
	}
	info, err := os.Lstat(stagingPath)
	if err != nil {
		return nil, nil, fmt.Errorf("server: stat socket %s: %w", socketPath, unwrapPathError(err))
	}
	if err := os.Link(stagingPath, socketPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, nil, fmt.Errorf("server: socket already in use: %s", socketPath)
		}
		// Filesystems without hard links fall back to rename, which cannot
		// detect a concurrent bind but still never exposes a permissive
		// socket.
		if renameErr := os.Rename(stagingPath, socketPath); renameErr != nil {
			return nil, nil, fmt.Errorf("server: move socket into place at %s: %w", socketPath, unwrapLinkError(renameErr))
		}
	}
	cleanup = false
	return ln, info, nil
}

// makeSocketStagingDir creates a private directory with a short random name
// so the staging socket path stays close to the final path's length.
func makeSocketStagingDir(parent string) (string, error) {
	var lastErr error
	for range 16 {
		var suffix [3]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", err
		}
		dir := filepath.Join(parent, ".g"+hex.EncodeToString(suffix[:]))
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			// Mkdir honors the umask; make the mode explicit.
			if err := os.Chmod(dir, 0o700); err != nil {
				_ = os.Remove(dir)
				return "", err
			}
			return dir, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}

func unwrapLinkError(err error) error {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}
	return err
}
