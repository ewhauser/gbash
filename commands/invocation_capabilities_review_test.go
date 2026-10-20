package commands

import (
	"context"
	"errors"
	"os"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
	"github.com/ewhauser/gbash/policy"
)

func TestCommandFSLinkRequiresWriteOnSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mem := gbfs.NewMemory()
	if err := mem.MkdirAll(ctx, "/ro", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mem.MkdirAll(ctx, "/tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := mem.OpenFile(ctx, "/ro/secret", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	inv := NewInvocation(&InvocationOptions{
		FileSystem: mem,
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/tmp"},
		}),
	})
	err = inv.FS.Link(ctx, "/ro/secret", "/tmp/alias")
	if !policy.IsDenied(err) {
		t.Fatalf("Link(read-only source) error = %v, want policy denial", err)
	}
	if _, statErr := mem.Lstat(ctx, "/tmp/alias"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Lstat(/tmp/alias) error = %v, want not-exist", statErr)
	}
}

func TestCommandFSOpenFileCapsWritesAtMaxFileBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mem := gbfs.NewMemory()
	inv := NewInvocation(&InvocationOptions{
		FileSystem: mem,
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits:     policy.Limits{MaxFileBytes: 4},
		}),
	})
	file, err := inv.FS.OpenFile(ctx, "/out", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if n, err := file.Write([]byte("hello")); n != 4 || !errors.Is(err, gbfs.ErrFileTooLarge) {
		t.Fatalf("Write() = %d, %v; want 4, EFBIG", n, err)
	}
}

func TestCommandFSResolveAppliesDotDotAfterSymlink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mem := gbfs.NewMemory()
	if err := mem.MkdirAll(ctx, "/a/b", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mem.Symlink(ctx, "/a/b", "/lnk"); err != nil {
		t.Fatal(err)
	}
	if err := mem.MkdirAll(ctx, "/a/data", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mem.MkdirAll(ctx, "/data", 0o755); err != nil {
		t.Fatal(err)
	}
	inv := NewInvocation(&InvocationOptions{FileSystem: mem, Cwd: "/"})
	if err := inv.FS.Remove(ctx, "/lnk/../data", true); err != nil {
		t.Fatalf("Remove(/lnk/../data) = %v", err)
	}
	if _, err := mem.Lstat(ctx, "/a/data"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("/a/data still exists: %v", err)
	}
	if _, err := mem.Lstat(ctx, "/data"); err != nil {
		t.Fatalf("top-level /data was removed: %v", err)
	}
}
