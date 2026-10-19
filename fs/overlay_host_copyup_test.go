//go:build !windows && !js

package fs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const overlayHostReadCap = 100

// newCappedHostOverlay mounts a host directory holding a file above the
// HostFS read cap under an overlay.
func newCappedHostOverlay(t *testing.T) (*OverlayFS, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "big"), []byte(strings.Repeat("a", 1000)), 0o640); err != nil {
		t.Fatalf("WriteFile(big) error = %v", err)
	}
	host, err := NewHost(HostOptions{Root: root, VirtualRoot: "/", MaxFileReadBytes: overlayHostReadCap})
	if err != nil {
		t.Fatalf("NewHost() error = %v", err)
	}
	return NewOverlay(host), root
}

func TestOverlayFSTruncatesLowerFileAboveReadCap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, root := newCappedHostOverlay(t)

	file, err := overlay.OpenFile(ctx, "/big", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(O_TRUNC) error = %v", err)
	}
	if _, err := io.WriteString(file, "x\n"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := readTestFile(t, overlay, "/big"); got != "x\n" {
		t.Fatalf("read after overwrite = %q, want %q", got, "x\n")
	}
	info, err := overlay.Stat(ctx, "/big")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("Mode().Perm() = %o, want the lower mode 640", got)
	}
	hostData, err := os.ReadFile(filepath.Join(root, "big"))
	if err != nil || len(hostData) != 1000 {
		t.Fatalf("host file changed: len=%d err=%v", len(hostData), err)
	}
}

func TestOverlayFSMetadataOpsAndRenameOnLowerFileAboveReadCap(t *testing.T) {
	t.Parallel()
	ops := []struct {
		name  string
		path  string
		apply func(context.Context, *OverlayFS) error
	}{
		{"Chmod", "/big", func(ctx context.Context, o *OverlayFS) error { return o.Chmod(ctx, "/big", 0o600) }},
		{"Chown", "/big", func(ctx context.Context, o *OverlayFS) error { return o.Chown(ctx, "/big", 5, 6, true) }},
		{"Chtimes", "/big", func(ctx context.Context, o *OverlayFS) error {
			return o.Chtimes(ctx, "/big", overlayCopyUpTime, overlayCopyUpTime)
		}},
		{"Link", "/big2", func(ctx context.Context, o *OverlayFS) error { return o.Link(ctx, "/big", "/big2") }},
		{"Rename", "/big2", func(ctx context.Context, o *OverlayFS) error { return o.Rename(ctx, "/big", "/big2") }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			overlay, _ := newCappedHostOverlay(t)

			if err := op.apply(ctx, overlay); err != nil {
				t.Fatalf("%s error = %v", op.name, err)
			}
			info, err := overlay.Stat(ctx, op.path)
			if err != nil {
				t.Fatalf("Stat(%s) error = %v", op.path, err)
			}
			if info.Size() != 1000 {
				t.Fatalf("Stat(%s).Size() = %d, want 1000", op.path, info.Size())
			}
			// The content still comes from the lower layer, so reading it hits
			// the same cap as reading the untouched lower file.
			_, err = overlay.Open(ctx, op.path)
			if !errors.Is(err, syscall.EFBIG) {
				t.Fatalf("Open(%s) error = %v, want file too large", op.path, err)
			}
		})
	}
}

func TestOverlayFSRenamedLowerFileKeepsContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "f"), []byte("lower\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	host, err := NewHost(HostOptions{Root: root, VirtualRoot: "/"})
	if err != nil {
		t.Fatalf("NewHost() error = %v", err)
	}
	overlay := NewOverlay(host)
	writeTestFile(t, overlay, "/d/u", "upper\n")

	if err := overlay.Rename(ctx, "/d", "/e"); err != nil {
		t.Fatalf("Rename(/d, /e) error = %v", err)
	}
	if got := readTestFile(t, overlay, "/e/f"); got != "lower\n" {
		t.Fatalf("read /e/f = %q, want %q", got, "lower\n")
	}
	if got := readTestFile(t, overlay, "/e/u"); got != "upper\n" {
		t.Fatalf("read /e/u = %q, want %q", got, "upper\n")
	}
	if _, err := overlay.Lstat(ctx, "/d"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(/d) error = %v, want not exist", err)
	}
}

func TestOverlayFSRenameUpperOnlyFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay := NewOverlay(NewMemory())
	writeTestFile(t, overlay, "/a", "a\n")
	if err := overlay.Link(ctx, "/a", "/a2"); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if err := overlay.Rename(ctx, "/a", "/b"); err != nil {
		t.Fatalf("Rename(/a, /b) error = %v", err)
	}
	if got := readTestFile(t, overlay, "/b"); got != "a\n" {
		t.Fatalf("read /b = %q, want %q", got, "a\n")
	}
	if _, err := overlay.Lstat(ctx, "/a"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(/a) error = %v, want not exist", err)
	}
	// The rename moves the node, so the hard link still shares content.
	writeTestFile(t, overlay, "/b", "b\n")
	if got := readTestFile(t, overlay, "/a2"); got != "b\n" {
		t.Fatalf("read /a2 = %q, want %q", got, "b\n")
	}
}

func TestOverlayFSCopyUpRereadsChangedLowerDirMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "inner"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	host, err := NewHost(HostOptions{Root: root, VirtualRoot: "/"})
	if err != nil {
		t.Fatalf("NewHost() error = %v", err)
	}
	overlay := NewOverlay(host)

	// Walk /sub as a path prefix so the overlay caches it.
	if _, err := overlay.Stat(ctx, "/sub/inner"); err != nil {
		t.Fatalf("Stat(/sub/inner) error = %v", err)
	}
	changed := time.Date(2002, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := os.Chmod(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	if err := os.Chtimes(filepath.Join(root, "sub"), changed, changed); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	writeTestFile(t, overlay, "/sub/inner/new", "x")
	info, err := overlay.Stat(ctx, "/sub")
	if err != nil {
		t.Fatalf("Stat(/sub) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("Stat(/sub).Mode().Perm() = %o, want the current lower mode 700", got)
	}
	if !info.ModTime().Equal(changed) {
		t.Fatalf("Stat(/sub).ModTime() = %v, want %v", info.ModTime(), changed)
	}
}
