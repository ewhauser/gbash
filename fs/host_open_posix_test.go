//go:build !windows && !js

package fs

import (
	"context"
	"errors"
	"io"
	stdfs "io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func newHostFIFOTestBackends(t *testing.T) map[string]func(root string) FileSystem {
	t.Helper()
	return map[string]func(root string) FileSystem{
		"ReadWriteFS": func(root string) FileSystem {
			fsys, err := NewReadWrite(ReadWriteOptions{Root: root})
			if err != nil {
				t.Fatalf("NewReadWrite() error = %v", err)
			}
			return fsys
		},
		"HostFS": func(root string) FileSystem {
			fsys, err := NewHost(HostOptions{Root: root, VirtualRoot: "/"})
			if err != nil {
				t.Fatalf("NewHost() error = %v", err)
			}
			return fsys
		},
	}
}

func makeHostFIFO(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(root, "p"), 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	return root
}

// runWithin fails the test if fn does not return within a generous bound, so a
// regression shows up as a failure instead of a hung test binary.
func runWithin(t *testing.T, what string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not return after the context ended", what)
		return nil
	}
}

func TestHostBackendsFIFOReadWithoutWriterStopsAtContextDeadline(t *testing.T) {
	t.Parallel()
	for name, newFS := range newHostFIFOTestBackends(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fsys := newFS(makeHostFIFO(t))
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			err := runWithin(t, "open+read", func() error {
				file, err := fsys.Open(ctx, "/p")
				if err != nil {
					return err
				}
				defer func() { _ = file.Close() }()
				_, err = file.Read(make([]byte, 8))
				return err
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Read() error = %v, want deadline exceeded", err)
			}
		})
	}
}

func TestReadWriteFSFIFOWriteWithoutReaderStopsAtContextCancel(t *testing.T) {
	t.Parallel()
	fsys, err := NewReadWrite(ReadWriteOptions{Root: makeHostFIFO(t)})
	if err != nil {
		t.Fatalf("NewReadWrite() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	err = runWithin(t, "open for write", func() error {
		file, err := fsys.OpenFile(ctx, "/p", os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		_ = file.Close()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenFile(O_WRONLY) error = %v, want canceled", err)
	}
}

func TestReadWriteFSFIFORoundTrip(t *testing.T) {
	t.Parallel()
	fsys, err := NewReadWrite(ReadWriteOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("NewReadWrite() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := fsys.Mkfifo(ctx, "/p", 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	writeErr := make(chan error, 1)
	go func() {
		// The writer's open waits for the reader, as open(2) would.
		file, err := fsys.OpenFile(ctx, "/p", os.O_WRONLY, 0)
		if err != nil {
			writeErr <- err
			return
		}
		_, err = io.WriteString(file, "hello\n")
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		writeErr <- err
	}()

	reader, err := fsys.Open(ctx, "/p")
	if err != nil {
		t.Fatalf("Open(reader) error = %v", err)
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got, want := string(data), "hello\n"; got != want {
		t.Fatalf("ReadAll() = %q, want %q", got, want)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("writer error = %v", err)
	}
}

func TestHostBackendsRefuseSockets(t *testing.T) {
	t.Parallel()
	for name, newFS := range newHostFIFOTestBackends(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Unix socket paths are length-limited, so keep the root short.
			root, err := os.MkdirTemp("", "gbs")
			if err != nil {
				t.Fatalf("MkdirTemp() error = %v", err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(root) })
			var lc net.ListenConfig
			listener, err := lc.Listen(context.Background(), "unix", filepath.Join(root, "sock"))
			if err != nil {
				t.Skipf("unix socket unavailable: %v", err)
			}
			t.Cleanup(func() { _ = listener.Close() })

			fsys := newFS(root)
			_, err = fsys.Open(context.Background(), "/sock")
			if !errors.Is(err, stdfs.ErrPermission) {
				t.Fatalf("Open(socket) error = %v, want permission denied", err)
			}
		})
	}
}

func TestHostOpenableMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode stdfs.FileMode
		want bool
	}{
		{0o644, true},
		{stdfs.ModeDir | 0o755, true},
		{stdfs.ModeNamedPipe | 0o600, true},
		{stdfs.ModeDevice | stdfs.ModeCharDevice | 0o666, false},
		{stdfs.ModeDevice | 0o660, false},
		{stdfs.ModeSocket | 0o755, false},
		{stdfs.ModeIrregular, false},
	}
	for _, tc := range cases {
		if got := hostOpenableMode(tc.mode); got != tc.want {
			t.Errorf("hostOpenableMode(%v) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}
