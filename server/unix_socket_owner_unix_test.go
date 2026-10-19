//go:build unix

package server

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnixListenerCloseKeepsReplacementSocket(t *testing.T) {
	t.Parallel()
	socket := newShortSocketPath(t)

	ln, err := listenUnixSocket(t.Context(), socket)
	if err != nil {
		t.Fatalf("listenUnixSocket(%q) error = %v", socket, err)
	}

	// Another server replaces the path while the first is still running.
	if err := os.Remove(socket); err != nil {
		t.Fatalf("Remove(%q) error = %v", socket, err)
	}
	other, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("Listen(replacement) error = %v", err)
	}
	defer func() { _ = other.Close() }()

	_ = ln.Close()
	if _, err := os.Lstat(socket); err != nil {
		t.Fatalf("replacement socket removed by first listener's Close: %v", err)
	}
	conn, err := net.Dial("unix", socket) //nolint:noctx // test helper
	if err != nil {
		t.Fatalf("Dial(replacement) error = %v", err)
	}
	_ = conn.Close()
}

func TestUnixListenerCloseRemovesOwnSocket(t *testing.T) {
	t.Parallel()
	socket := newShortSocketPath(t)

	ln, err := listenUnixSocket(t.Context(), socket)
	if err != nil {
		t.Fatalf("listenUnixSocket(%q) error = %v", socket, err)
	}
	_ = ln.Close()
	if _, err := os.Lstat(socket); !os.IsNotExist(err) {
		t.Fatalf("Lstat(%q) after Close error = %v, want not exist", socket, err)
	}
}

func TestListenPrivateUnixSocketDoesNotReplaceSocketBoundConcurrently(t *testing.T) {
	t.Parallel()
	socket := newShortSocketPath(t)

	// Simulate a server that bound the path after the stale-socket probe.
	other, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("Listen(other) error = %v", err)
	}
	defer func() { _ = other.Close() }()

	ln, _, err := listenPrivateUnixSocket(t.Context(), socket)
	if err == nil {
		_ = ln.Close()
		t.Fatal("listenPrivateUnixSocket() error = nil, want socket already in use")
	}
	if !strings.Contains(err.Error(), "socket already in use") {
		t.Fatalf("error = %v, want socket already in use", err)
	}
	conn, err := net.Dial("unix", socket) //nolint:noctx // test helper
	if err != nil {
		t.Fatalf("Dial(other) error = %v; the existing socket must stay in place", err)
	}
	_ = conn.Close()
	entries, err := os.ReadDir(filepath.Dir(socket))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".g") && entry.IsDir() {
			if _, statErr := os.Stat(filepath.Join(filepath.Dir(socket), entry.Name(), "s")); statErr == nil {
				t.Fatalf("staging socket left behind in %s", entry.Name())
			}
		}
	}
}

func TestListenUnixSocketPathLengthErrorsNameFinalPath(t *testing.T) {
	t.Parallel()
	base, err := os.MkdirTemp("", "gbl-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })

	tooLong := filepath.Join(base, strings.Repeat("x", maxUnixSocketPathLen))
	_, err = listenUnixSocket(t.Context(), tooLong)
	if err == nil || !strings.Contains(err.Error(), "byte limit") || !strings.Contains(err.Error(), tooLong) {
		t.Fatalf("listenUnixSocket(too long) error = %v, want length error naming the socket path", err)
	}

	// A path that fits but leaves no room for the staging directory.
	dir := base
	for len(dir) < maxUnixSocketPathLen-10 {
		dir = filepath.Join(dir, "d")
	}
	tight := filepath.Join(dir, "s")
	if len(tight) > maxUnixSocketPathLen {
		t.Skipf("could not build a tight path: %d bytes", len(tight))
	}
	_, err = listenUnixSocket(t.Context(), tight)
	if err == nil {
		t.Fatal("listenUnixSocket(tight) error = nil, want staging length error")
	}
	if !strings.Contains(err.Error(), tight) || strings.Contains(err.Error(), "/.g") {
		t.Fatalf("listenUnixSocket(tight) error = %v, want error naming the final socket path only", err)
	}
}
