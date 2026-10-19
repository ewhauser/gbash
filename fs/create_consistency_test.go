package fs

import (
	"context"
	"errors"
	stdfs "io/fs"
	"os"
	"syscall"
	"testing"
)

// mutableTreeBackends are the in-memory backends that must share creating and
// mode semantics.
func mutableTreeBackends() map[string]func() FileSystem {
	return map[string]func() FileSystem{
		"MemoryFS":  func() FileSystem { return NewMemory() },
		"TrieFS":    func() FileSystem { return NewTrie() },
		"OverlayFS": func() FileSystem { return NewOverlay(NewMemory()) },
	}
}

func TestCreatingOperationsRequireExistingParentAcrossBackends(t *testing.T) {
	t.Parallel()
	for name, newFS := range mutableTreeBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fsys := newFS()
			writeTestFile(t, fsys, "/file", "data\n")

			if _, err := fsys.OpenFile(ctx, "/nope/x", os.O_CREATE|os.O_WRONLY, 0o644); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("OpenFile(/nope/x) error = %v, want not exist", err)
			}
			if _, err := fsys.OpenFile(ctx, "/file/x", os.O_CREATE|os.O_WRONLY, 0o644); !errors.Is(err, syscall.ENOTDIR) {
				t.Fatalf("OpenFile(/file/x) error = %v, want ENOTDIR", err)
			}
			if err := fsys.Symlink(ctx, "/file", "/nope/link"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("Symlink(/nope/link) error = %v, want not exist", err)
			}
			if err := fsys.Link(ctx, "/file", "/nope/hard"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("Link(/nope/hard) error = %v, want not exist", err)
			}
			if err := fsys.Rename(ctx, "/file", "/nope/moved"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("Rename(/nope/moved) error = %v, want not exist", err)
			}
			if _, err := fsys.Lstat(ctx, "/nope"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("Lstat(/nope) error = %v, want not exist (parent must not be created)", err)
			}
			if got := readTestFile(t, fsys, "/file"); got != "data\n" {
				t.Fatalf("read /file = %q, want source kept after failed rename", got)
			}
		})
	}
}

func TestRenameIntoOwnSubtreeLeavesNoDebrisAcrossBackends(t *testing.T) {
	t.Parallel()
	for name, newFS := range mutableTreeBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fsys := newFS()
			writeTestFile(t, fsys, "/a/file.txt", "keep\n")

			if err := fsys.Rename(ctx, "/a", "/a/x/y"); err == nil {
				t.Fatal("Rename(/a, /a/x/y) error = nil, want failure")
			}
			if _, err := fsys.Lstat(ctx, "/a/x"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("Lstat(/a/x) error = %v, want not exist after failed rename", err)
			}
			if err := fsys.Rename(ctx, "/a", "/a/x"); !errors.Is(err, stdfs.ErrInvalid) {
				t.Fatalf("Rename(/a, /a/x) error = %v, want EINVAL", err)
			}
			if got := readTestFile(t, fsys, "/a/file.txt"); got != "keep\n" {
				t.Fatalf("read /a/file.txt = %q, want tree intact", got)
			}
		})
	}
}

func TestChmodPreservesSpecialModeBitsAcrossBackends(t *testing.T) {
	t.Parallel()
	for name, newFS := range mutableTreeBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fsys := newFS()
			writeTestFile(t, fsys, "/f", "x")
			want := stdfs.ModeSetuid | stdfs.ModeSetgid | stdfs.ModeSticky | 0o755
			if err := fsys.Chmod(ctx, "/f", want|stdfs.ModeSymlink); err != nil {
				t.Fatalf("Chmod() error = %v", err)
			}
			info, err := fsys.Lstat(ctx, "/f")
			if err != nil {
				t.Fatalf("Lstat() error = %v", err)
			}
			if got := info.Mode(); got != want {
				t.Fatalf("Mode() = %v, want %v", got, want)
			}
		})
	}
}

func TestCallerPermCannotInjectTypeBitsAcrossBackends(t *testing.T) {
	t.Parallel()
	for name, newFS := range mutableTreeBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fsys := newFS()

			// nodejs fs.mkdirSync(p, {mode: 134217728 + 493}) passes ModeSymlink|0755.
			if err := fsys.MkdirAll(ctx, "/d", stdfs.ModeSymlink|0o755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			info, err := fsys.Lstat(ctx, "/d")
			if err != nil {
				t.Fatalf("Lstat(/d) error = %v", err)
			}
			if got, want := info.Mode(), stdfs.ModeDir|0o755; got != want {
				t.Fatalf("Lstat(/d).Mode() = %v, want %v", got, want)
			}

			file, err := fsys.OpenFile(ctx, "/d/f", os.O_CREATE|os.O_WRONLY, stdfs.ModeSymlink|stdfs.ModeDir|stdfs.ModeSetuid|0o640)
			if err != nil {
				t.Fatalf("OpenFile(/d/f) error = %v", err)
			}
			_ = file.Close()
			info, err = fsys.Lstat(ctx, "/d/f")
			if err != nil {
				t.Fatalf("Lstat(/d/f) error = %v", err)
			}
			if got, want := info.Mode(), stdfs.ModeSetuid|0o640; got != want {
				t.Fatalf("Lstat(/d/f).Mode() = %v, want %v", got, want)
			}

			if err := fsys.Remove(ctx, "/d", true); err != nil {
				t.Fatalf("Remove(/d) error = %v", err)
			}
			if _, err := fsys.Lstat(ctx, "/d"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("Lstat(/d) after remove error = %v, want not exist", err)
			}
		})
	}
}
