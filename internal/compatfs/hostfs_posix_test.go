//go:build !windows

package compatfs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestHostFSReadsWritesAndResolvesSymlinks(t *testing.T) {
	t.Chdir(t.TempDir())

	fsys, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	file, err := fsys.OpenFile(context.Background(), "note.txt", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if _, err := io.WriteString(file, "hello\n"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := fsys.MkdirAll(context.Background(), "sub", 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := fsys.Rename(context.Background(), "note.txt", "sub/note.txt"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if err := fsys.Symlink(context.Background(), "sub/note.txt", "link.txt"); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	entries, err := fsys.ReadDir(context.Background(), "sub")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "note.txt" {
		t.Fatalf("ReadDir() = %#v, want single note.txt entry", entries)
	}

	target, err := fsys.Readlink(context.Background(), "link.txt")
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got, want := target, "sub/note.txt"; got != want {
		t.Fatalf("Readlink() = %q, want %q", got, want)
	}

	resolved, err := fsys.Realpath(context.Background(), "link.txt")
	if err != nil {
		t.Fatalf("Realpath() error = %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(tmpDirFromFS(t, fsys))
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	wantResolved := filepath.ToSlash(filepath.Join(canonicalRoot, "sub", "note.txt"))
	if resolved != wantResolved {
		t.Fatalf("Realpath() = %q, want %q", resolved, wantResolved)
	}

	reader, err := fsys.Open(context.Background(), "sub/note.txt")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close(reader) error = %v", err)
	}
	if got, want := string(data), "hello\n"; got != want {
		t.Fatalf("contents = %q, want %q", got, want)
	}
}

func TestHostFSStatPreservesRawSysStat(t *testing.T) {
	t.Chdir(t.TempDir())

	fsys, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDirFromFS(t, fsys), "note.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	info, err := fsys.Stat(context.Background(), "note.txt")
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if _, ok := info.Sys().(*syscall.Stat_t); !ok {
		t.Fatalf("Stat().Sys() = %T, want *syscall.Stat_t", info.Sys())
	}
}

func TestHostFSChdirAllowsCurrentLongPath(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})

	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(root) error = %v", err)
	}

	segment := strings.Repeat("z", 31)
	for depth := range 256 {
		if err := os.Mkdir(segment, 0o755); err != nil {
			t.Fatalf("Mkdir(depth=%d) error = %v", depth, err)
		}
		if err := os.Chdir(segment); err != nil {
			t.Fatalf("Chdir(depth=%d) error = %v", depth, err)
		}
	}

	fsys, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	current := fsys.Getwd()
	if len(filepath.FromSlash(current)) <= 1024 {
		t.Fatalf("Getwd() = %q, want path longer than PATH_MAX-ish threshold", current)
	}

	if err := fsys.Chdir(current); err != nil {
		t.Fatalf("Chdir(current long path) error = %v", err)
	}
	realpath, err := fsys.Realpath(context.Background(), ".")
	if err != nil {
		t.Fatalf("Realpath(.) error = %v", err)
	}
	if got, want := realpath, current; got != want {
		t.Fatalf("Realpath(.) = %q, want %q", got, want)
	}
}

func tmpDirFromFS(t *testing.T, fsys *HostFS) string {
	t.Helper()
	return filepath.FromSlash(fsys.Getwd())
}
