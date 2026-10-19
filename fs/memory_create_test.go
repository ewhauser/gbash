package fs

import (
	"context"
	"errors"
	stdfs "io/fs"
	"os"
	"syscall"
	"testing"
)

func TestMemoryFSOpenFileCreateRequiresExistingParent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		flag int
	}{
		{name: "truncate", flag: os.O_CREATE | os.O_WRONLY | os.O_TRUNC},
		{name: "append", flag: os.O_CREATE | os.O_WRONLY | os.O_APPEND},
		{name: "exclusive", flag: os.O_CREATE | os.O_WRONLY | os.O_EXCL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			mem := NewMemory()

			_, err := mem.OpenFile(ctx, "/nope/dir/x", tc.flag, 0o644)
			if !errors.Is(err, syscall.ENOENT) || !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("OpenFile(/nope/dir/x) error = %v, want ENOENT", err)
			}
			if _, err := mem.Lstat(ctx, "/nope"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("Lstat(/nope) error = %v, want not exist (parent must not be created)", err)
			}

			writeTestFile(t, mem, "/file", "data\n")
			_, err = mem.OpenFile(ctx, "/file/x", tc.flag, 0o644)
			if !errors.Is(err, syscall.ENOTDIR) {
				t.Fatalf("OpenFile(/file/x) error = %v, want ENOTDIR", err)
			}
			_, err = mem.OpenFile(ctx, "/file/sub/x", tc.flag, 0o644)
			if !errors.Is(err, syscall.ENOTDIR) {
				t.Fatalf("OpenFile(/file/sub/x) error = %v, want ENOTDIR", err)
			}
		})
	}
}

func TestMemoryFSOpenFileCreateFollowsSymlinkedParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mem := NewMemory()
	if err := mem.MkdirAll(ctx, "/real", 0o755); err != nil {
		t.Fatalf("MkdirAll(/real) error = %v", err)
	}
	if err := mem.Symlink(ctx, "/real", "/alias"); err != nil {
		t.Fatalf("Symlink(/alias) error = %v", err)
	}
	writeTestFile(t, mem, "/alias/file.txt", "hello\n")
	if _, err := mem.Stat(ctx, "/real/file.txt"); err != nil {
		t.Fatalf("Stat(/real/file.txt) error = %v", err)
	}

	if err := mem.Symlink(ctx, "/missing", "/dangling"); err != nil {
		t.Fatalf("Symlink(/dangling) error = %v", err)
	}
	if _, err := mem.OpenFile(ctx, "/dangling/file.txt", os.O_CREATE|os.O_WRONLY, 0o644); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("OpenFile(/dangling/file.txt) error = %v, want not exist", err)
	}
	if _, err := mem.Lstat(ctx, "/missing"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Lstat(/missing) error = %v, want not exist", err)
	}
}

func TestMemoryFSCreatingOperationsRequireExistingParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mem := NewMemory()
	writeTestFile(t, mem, "/file", "data\n")

	if err := mem.Symlink(ctx, "/file", "/nope/link"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Symlink(/nope/link) error = %v, want not exist", err)
	}
	if err := mem.Link(ctx, "/file", "/nope/hard"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Link(/nope/hard) error = %v, want not exist", err)
	}
	if err := mem.Rename(ctx, "/file", "/nope/moved"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Rename(/nope/moved) error = %v, want not exist", err)
	}
	if _, err := mem.Lstat(ctx, "/nope"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Lstat(/nope) error = %v, want not exist", err)
	}
	if _, err := mem.Stat(ctx, "/file"); err != nil {
		t.Fatalf("Stat(/file) error = %v, want source kept after failed rename", err)
	}
}

func TestMemoryFSRenameDirectoryIntoOwnSubtreeFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mem := NewMemory()
	writeTestFile(t, mem, "/a/b/file.txt", "keep\n")
	if err := mem.Symlink(ctx, "/a", "/alias"); err != nil {
		t.Fatalf("Symlink(/alias) error = %v", err)
	}

	for _, dst := range []string{"/a/b/c", "/a/c", "/alias/c", "/a/b/../c"} {
		if err := mem.Rename(ctx, "/a", dst); !errors.Is(err, stdfs.ErrInvalid) {
			t.Fatalf("Rename(/a, %s) error = %v, want EINVAL", dst, err)
		}
	}

	if got := readTestFile(t, mem, "/a/b/file.txt"); got != "keep\n" {
		t.Fatalf("ReadFile(/a/b/file.txt) = %q, want tree intact", got)
	}
	entries, err := mem.ReadDir(ctx, "/")
	if err != nil {
		t.Fatalf("ReadDir(/) error = %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if got, want := len(names), 2; got != want {
		t.Fatalf("ReadDir(/) = %v, want [a alias]", names)
	}

	// A sibling whose name shares the prefix is not a descendant.
	if err := mem.Rename(ctx, "/a", "/ab"); err != nil {
		t.Fatalf("Rename(/a, /ab) error = %v", err)
	}
	if _, err := mem.Stat(ctx, "/ab/b/file.txt"); err != nil {
		t.Fatalf("Stat(/ab/b/file.txt) error = %v", err)
	}
}

func TestMemoryFSLinkThroughSymlinkedParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mem := NewMemory()
	writeTestFile(t, mem, "/file", "data\n")
	if err := mem.MkdirAll(ctx, "/dir", 0o755); err != nil {
		t.Fatalf("MkdirAll(/dir) error = %v", err)
	}
	if err := mem.Symlink(ctx, "/dir", "/alias"); err != nil {
		t.Fatalf("Symlink(/alias) error = %v", err)
	}

	if err := mem.Link(ctx, "/file", "/alias/hard"); err != nil {
		t.Fatalf("Link(/file, /alias/hard) error = %v", err)
	}
	if got := readTestFile(t, mem, "/dir/hard"); got != "data\n" {
		t.Fatalf("ReadFile(/dir/hard) = %q, want linked contents", got)
	}
	if err := mem.Link(ctx, "/file", "/alias/hard"); !errors.Is(err, stdfs.ErrExist) {
		t.Fatalf("second Link(/alias/hard) error = %v, want exist", err)
	}
}

func TestOverlayFSCreateStillCopiesUpLowerOnlyParents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lower := seededMemory(t, map[string]string{"/lower/dir/existing.txt": "lower\n"})
	overlay := NewOverlay(lower)

	writeTestFile(t, overlay, "/lower/dir/new.txt", "upper\n")
	if got := readTestFile(t, overlay, "/lower/dir/new.txt"); got != "upper\n" {
		t.Fatalf("ReadFile(/lower/dir/new.txt) = %q", got)
	}

	if err := overlay.Chmod(ctx, "/lower/dir/existing.txt", 0o600); err != nil {
		t.Fatalf("Chmod(copy-up) error = %v", err)
	}
	if err := overlay.Rename(ctx, "/lower/dir/existing.txt", "/lower/moved.txt"); err != nil {
		t.Fatalf("Rename(copy-up) error = %v", err)
	}
	if _, err := lower.Stat(ctx, "/lower/moved.txt"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("lower Stat(/lower/moved.txt) error = %v, want lower untouched", err)
	}
}
