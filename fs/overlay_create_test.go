package fs

import (
	"context"
	"errors"
	stdfs "io/fs"
	"os"
	"syscall"
	"testing"
)

func TestOverlayFSOpenFileCreateRequiresExistingParent(t *testing.T) {
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
			overlay, _ := newSymlinkOverlay(t)
			writeTestFile(t, overlay, "/w/upper-file", "data\n")

			for _, name := range []string{"/nope/dir/x", "/nope/x", "/w/nope/x"} {
				if _, err := overlay.OpenFile(ctx, name, tc.flag, 0o644); !errors.Is(err, stdfs.ErrNotExist) {
					t.Fatalf("OpenFile(%q) error = %v, want not exist", name, err)
				}
			}
			for _, name := range []string{"/nope", "/w/nope"} {
				if _, err := overlay.Lstat(ctx, name); !errors.Is(err, stdfs.ErrNotExist) {
					t.Fatalf("Lstat(%q) error = %v, want not exist (parent must not be created)", name, err)
				}
			}

			for _, name := range []string{"/w/real/f/x", "/w/real/f/sub/x", "/w/upper-file/x", "/w/flink/x"} {
				if _, err := overlay.OpenFile(ctx, name, tc.flag, 0o644); !errors.Is(err, syscall.ENOTDIR) {
					t.Fatalf("OpenFile(%q) error = %v, want ENOTDIR", name, err)
				}
			}

			// A parent that exists only in the lower layer is still a valid
			// destination: the upper layer materializes it during copy-up.
			file, err := overlay.OpenFile(ctx, "/w/real/new", tc.flag, 0o644)
			if err != nil {
				t.Fatalf("OpenFile(/w/real/new) error = %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("Close(/w/real/new) error = %v", err)
			}
		})
	}
}

func TestOverlayFSOpenFileCreateFollowsSymlinkedParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, lower := newSymlinkOverlay(t)
	if err := lower.Symlink(ctx, "missing", "/w/dangling-dir"); err != nil {
		t.Fatalf("Symlink(dangling-dir) error = %v", err)
	}
	if err := lower.Symlink(ctx, "/nope/x", "/w/dangling-deep"); err != nil {
		t.Fatalf("Symlink(dangling-deep) error = %v", err)
	}

	file, err := overlay.OpenFile(ctx, "/w/link/new", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(/w/link/new) error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(/w/link/new) error = %v", err)
	}
	if _, err := overlay.Stat(ctx, "/w/real/new"); err != nil {
		t.Fatalf("Stat(/w/real/new) error = %v", err)
	}

	// The parent is a dangling link, and a final dangling link whose target's
	// parent is missing: both fail without materializing the target.
	for _, name := range []string{"/w/dangling-dir/x", "/w/dangling-deep"} {
		if _, err := overlay.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY, 0o644); !errors.Is(err, stdfs.ErrNotExist) {
			t.Fatalf("OpenFile(%q) error = %v, want not exist", name, err)
		}
	}
	for _, name := range []string{"/w/missing", "/nope"} {
		if _, err := overlay.Lstat(ctx, name); !errors.Is(err, stdfs.ErrNotExist) {
			t.Fatalf("Lstat(%q) error = %v, want not exist", name, err)
		}
	}
}

func TestOverlayFSOpenFileCreateUnderRemovedLowerDirFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, _ := newSymlinkOverlay(t)
	if err := overlay.Remove(ctx, "/w/real", true); err != nil {
		t.Fatalf("Remove(/w/real) error = %v", err)
	}
	if _, err := overlay.OpenFile(ctx, "/w/real/x", os.O_CREATE|os.O_WRONLY, 0o644); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("OpenFile(/w/real/x) error = %v, want not exist", err)
	}
	if _, err := overlay.Lstat(ctx, "/w/real"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Lstat(/w/real) error = %v, want not exist (tombstone must stay)", err)
	}
}

func TestOverlayFSCreatingOperationsRequireExistingParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, _ := newSymlinkOverlay(t)

	ops := []struct {
		name string
		run  func(dst string) error
	}{
		{name: "Symlink", run: func(dst string) error { return overlay.Symlink(ctx, "/w/real/f", dst) }},
		{name: "Link", run: func(dst string) error { return overlay.Link(ctx, "/w/real/f", dst) }},
		{name: "Rename", run: func(dst string) error { return overlay.Rename(ctx, "/w/real/f", dst) }},
		{name: "Mkfifo", run: func(dst string) error { return overlay.Mkfifo(ctx, dst, 0o644) }},
	}
	for _, op := range ops {
		if err := op.run("/nope/x"); !errors.Is(err, stdfs.ErrNotExist) {
			t.Fatalf("%s(/nope/x) error = %v, want not exist", op.name, err)
		}
		if err := op.run("/w/real/f/x"); !errors.Is(err, syscall.ENOTDIR) {
			t.Fatalf("%s(/w/real/f/x) error = %v, want ENOTDIR", op.name, err)
		}
		if err := op.run("/w/flink/x"); !errors.Is(err, syscall.ENOTDIR) {
			t.Fatalf("%s(/w/flink/x) error = %v, want ENOTDIR", op.name, err)
		}
	}
	if _, err := overlay.Lstat(ctx, "/nope"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Lstat(/nope) error = %v, want not exist (parent must not be created)", err)
	}
	if got, want := readTestFile(t, overlay, "/w/real/f"), "orig"; got != want {
		t.Fatalf("read /w/real/f = %q, want %q", got, want)
	}
}
