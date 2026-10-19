package fs

import (
	"context"
	"errors"
	stdfs "io/fs"
	"os"
	"slices"
	"testing"
	"time"
)

// overlayMetadataOps applies one metadata change to name and reports a check
// that the change is visible in the merged view.
var overlayMetadataOps = []struct {
	name   string
	apply  func(context.Context, *OverlayFS, string) error
	verify func(*testing.T, stdfs.FileInfo)
}{
	{
		name:  "Chmod",
		apply: func(ctx context.Context, o *OverlayFS, name string) error { return o.Chmod(ctx, name, 0o700) },
		verify: func(t *testing.T, info stdfs.FileInfo) {
			t.Helper()
			if got := info.Mode().Perm(); got != 0o700 {
				t.Fatalf("Mode().Perm() = %o, want 700", got)
			}
		},
	},
	{
		name:  "Chown",
		apply: func(ctx context.Context, o *OverlayFS, name string) error { return o.Chown(ctx, name, 42, 43, true) },
		verify: func(t *testing.T, info stdfs.FileInfo) {
			t.Helper()
			ownership, ok := OwnershipFromFileInfo(info)
			if !ok || ownership.UID != 42 || ownership.GID != 43 {
				t.Fatalf("ownership = %+v (ok=%v), want 42:43", ownership, ok)
			}
		},
	},
	{
		name: "Chtimes",
		apply: func(ctx context.Context, o *OverlayFS, name string) error {
			return o.Chtimes(ctx, name, overlayCopyUpTime, overlayCopyUpTime)
		},
		verify: func(t *testing.T, info stdfs.FileInfo) {
			t.Helper()
			if !info.ModTime().Equal(overlayCopyUpTime) {
				t.Fatalf("ModTime() = %v, want %v", info.ModTime(), overlayCopyUpTime)
			}
		},
	},
	{
		name: "Lchtimes",
		apply: func(ctx context.Context, o *OverlayFS, name string) error {
			return o.Lchtimes(ctx, name, overlayCopyUpTime, overlayCopyUpTime)
		},
		verify: func(t *testing.T, info stdfs.FileInfo) {
			t.Helper()
			if !info.ModTime().Equal(overlayCopyUpTime) {
				t.Fatalf("ModTime() = %v, want %v", info.ModTime(), overlayCopyUpTime)
			}
		},
	},
}

var overlayCopyUpTime = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

func overlayDirNames(t *testing.T, fsys FileSystem, name string) []string {
	t.Helper()
	entries, err := fsys.ReadDir(context.Background(), name)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", name, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestOverlayFSDirMetadataCopyUpKeepsTombstones(t *testing.T) {
	t.Parallel()
	for _, op := range overlayMetadataOps {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			overlay := NewOverlay(seededMemory(t, map[string]string{
				"/d/a":     "a",
				"/d/b":     "b",
				"/d/sub/c": "c",
			}))
			if err := overlay.Remove(ctx, "/d/a", false); err != nil {
				t.Fatalf("Remove(/d/a) error = %v", err)
			}
			if err := overlay.Remove(ctx, "/d/sub/c", false); err != nil {
				t.Fatalf("Remove(/d/sub/c) error = %v", err)
			}

			if err := op.apply(ctx, overlay, "/d"); err != nil {
				t.Fatalf("%s(/d) error = %v", op.name, err)
			}

			info, err := overlay.Stat(ctx, "/d")
			if err != nil {
				t.Fatalf("Stat(/d) error = %v", err)
			}
			op.verify(t, info)
			for _, name := range []string{"/d/a", "/d/sub/c"} {
				if _, err := overlay.Lstat(ctx, name); !errors.Is(err, stdfs.ErrNotExist) {
					t.Fatalf("Lstat(%q) error = %v, want ErrNotExist", name, err)
				}
			}
			if got, want := overlayDirNames(t, overlay, "/d"), []string{"b", "sub"}; !slices.Equal(got, want) {
				t.Fatalf("ReadDir(/d) = %v, want %v", got, want)
			}
			if got := overlayDirNames(t, overlay, "/d/sub"); len(got) != 0 {
				t.Fatalf("ReadDir(/d/sub) = %v, want empty", got)
			}
			if got := readTestFile(t, overlay, "/d/b"); got != "b" {
				t.Fatalf("ReadFile(/d/b) = %q, want b", got)
			}
		})
	}
}

func TestOverlayFSDirMetadataCopyUpCopiesOnlyDirectoryNode(t *testing.T) {
	t.Parallel()
	for _, op := range overlayMetadataOps {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			lower := seededMemory(t, map[string]string{
				"/p/d/a":     "a",
				"/p/d/sub/c": "c",
			})
			if err := lower.Chmod(ctx, "/p", 0o750); err != nil {
				t.Fatalf("lower Chmod(/p) error = %v", err)
			}
			if err := lower.Chown(ctx, "/p/d", 7, 8, true); err != nil {
				t.Fatalf("lower Chown(/p/d) error = %v", err)
			}
			lowerTime := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
			if err := lower.Chtimes(ctx, "/p/d", lowerTime, lowerTime); err != nil {
				t.Fatalf("lower Chtimes(/p/d) error = %v", err)
			}
			overlay := NewOverlay(lower)

			if err := op.apply(ctx, overlay, "/p/d"); err != nil {
				t.Fatalf("%s(/p/d) error = %v", op.name, err)
			}

			for _, name := range []string{"/p/d/a", "/p/d/sub"} {
				if _, err := overlay.upper.Lstat(ctx, name); !errors.Is(err, stdfs.ErrNotExist) {
					t.Fatalf("upper Lstat(%q) error = %v, want ErrNotExist (children must stay in lower)", name, err)
				}
			}
			info, err := overlay.Stat(ctx, "/p/d")
			if err != nil {
				t.Fatalf("Stat(/p/d) error = %v", err)
			}
			op.verify(t, info)
			// Attributes the operation did not touch keep their lower values.
			if op.name != "Chown" {
				if ownership, ok := OwnershipFromFileInfo(info); !ok || ownership.UID != 7 || ownership.GID != 8 {
					t.Fatalf("Stat(/p/d) ownership = %+v (ok=%v), want 7:8", ownership, ok)
				}
			}
			// MemoryFS.Chmod stamps the modification time, so only Chown
			// leaves the lower mtime in place.
			if op.name == "Chown" && !info.ModTime().Equal(lowerTime) {
				t.Fatalf("Stat(/p/d).ModTime() = %v, want %v", info.ModTime(), lowerTime)
			}
			if op.name != "Chmod" && info.Mode().Perm() != 0o755 {
				t.Fatalf("Stat(/p/d).Mode().Perm() = %o, want 755", info.Mode().Perm())
			}
			parent, err := overlay.Stat(ctx, "/p")
			if err != nil {
				t.Fatalf("Stat(/p) error = %v", err)
			}
			if got := parent.Mode().Perm(); got != 0o750 {
				t.Fatalf("Stat(/p).Mode().Perm() = %o, want 750", got)
			}
			if got, want := overlayDirNames(t, overlay, "/p/d"), []string{"a", "sub"}; !slices.Equal(got, want) {
				t.Fatalf("ReadDir(/p/d) = %v, want %v", got, want)
			}
			if got := readTestFile(t, overlay, "/p/d/sub/c"); got != "c" {
				t.Fatalf("ReadFile(/p/d/sub/c) = %q, want c", got)
			}
		})
	}
}

// Rename clones from the merged view, so a renamed lower directory must carry
// neither children deleted before the rename nor copies of its old metadata.
func TestOverlayFSRenameLowerDirRespectsTombstones(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay := NewOverlay(seededMemory(t, map[string]string{
		"/d/a": "a",
		"/d/b": "b",
	}))
	if err := overlay.Remove(ctx, "/d/a", false); err != nil {
		t.Fatalf("Remove(/d/a) error = %v", err)
	}
	if err := overlay.Chmod(ctx, "/d", 0o700); err != nil {
		t.Fatalf("Chmod(/d) error = %v", err)
	}
	if err := overlay.Rename(ctx, "/d", "/e"); err != nil {
		t.Fatalf("Rename(/d, /e) error = %v", err)
	}
	if got, want := overlayDirNames(t, overlay, "/e"), []string{"b"}; !slices.Equal(got, want) {
		t.Fatalf("ReadDir(/e) = %v, want %v", got, want)
	}
	info, err := overlay.Stat(ctx, "/e")
	if err != nil {
		t.Fatalf("Stat(/e) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("Stat(/e).Mode().Perm() = %o, want 700", got)
	}
	if _, err := overlay.Lstat(ctx, "/d"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Lstat(/d) error = %v, want ErrNotExist", err)
	}
}

// Link copies up only the linked file; a later chmod of its lower parent must
// not disturb either name.
func TestOverlayFSLinkAfterDirMetadataCopyUp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay := NewOverlay(seededMemory(t, map[string]string{
		"/d/a": "a",
		"/d/b": "b",
	}))
	if err := overlay.Link(ctx, "/d/a", "/d/hard"); err != nil {
		t.Fatalf("Link(/d/a, /d/hard) error = %v", err)
	}
	if err := overlay.Chmod(ctx, "/d", 0o700); err != nil {
		t.Fatalf("Chmod(/d) error = %v", err)
	}
	if got, want := overlayDirNames(t, overlay, "/d"), []string{"a", "b", "hard"}; !slices.Equal(got, want) {
		t.Fatalf("ReadDir(/d) = %v, want %v", got, want)
	}
	for _, name := range []string{"/d/a", "/d/hard"} {
		if got := readTestFile(t, overlay, name); got != "a" {
			t.Fatalf("ReadFile(%q) = %q, want a", name, got)
		}
	}
	if _, err := overlay.upper.Lstat(ctx, "/d/b"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("upper Lstat(/d/b) error = %v, want ErrNotExist", err)
	}
}

// Every copy-up must create missing upper ancestors with their lower
// metadata; otherwise default-metadata upper directories shadow the lower ones.
func TestOverlayFSCopyUpPreservesAncestorMetadata(t *testing.T) {
	t.Parallel()
	ops := []struct {
		name  string
		apply func(context.Context, *OverlayFS) error
	}{
		{
			name: "OpenFileWriteOnly",
			apply: func(ctx context.Context, o *OverlayFS) error {
				return overlayWriteFile(ctx, o, "/p/d/f", os.O_WRONLY)
			},
		},
		{
			name: "OpenFileAppend",
			apply: func(ctx context.Context, o *OverlayFS) error {
				return overlayWriteFile(ctx, o, "/p/d/f", os.O_WRONLY|os.O_APPEND)
			},
		},
		{
			name:  "Link",
			apply: func(ctx context.Context, o *OverlayFS) error { return o.Link(ctx, "/p/d/f", "/p/d/g") },
		},
		{
			name: "OpenFileCreate",
			apply: func(ctx context.Context, o *OverlayFS) error {
				return overlayWriteFile(ctx, o, "/p/d/new", os.O_WRONLY|os.O_CREATE|os.O_EXCL)
			},
		},
		{
			name:  "Symlink",
			apply: func(ctx context.Context, o *OverlayFS) error { return o.Symlink(ctx, "f", "/p/d/l") },
		},
		{
			name:  "Mkfifo",
			apply: func(ctx context.Context, o *OverlayFS) error { return o.Mkfifo(ctx, "/p/d/q", 0o600) },
		},
		{
			name:  "MkdirAll",
			apply: func(ctx context.Context, o *OverlayFS) error { return o.MkdirAll(ctx, "/p/d/x/y", 0o755) },
		},
		{
			name:  "Rename",
			apply: func(ctx context.Context, o *OverlayFS) error { return o.Rename(ctx, "/p/d/f", "/p/d/h") },
		},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			lower := seededMemory(t, map[string]string{"/p/d/f": "f"})
			if err := lower.Chmod(ctx, "/p", 0o750); err != nil {
				t.Fatalf("lower Chmod(/p) error = %v", err)
			}
			if err := lower.Chown(ctx, "/p/d", 7, 8, true); err != nil {
				t.Fatalf("lower Chown(/p/d) error = %v", err)
			}
			lowerTime := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
			if err := lower.Chtimes(ctx, "/p", lowerTime, lowerTime); err != nil {
				t.Fatalf("lower Chtimes(/p) error = %v", err)
			}
			overlay := NewOverlay(lower)

			if err := op.apply(ctx, overlay); err != nil {
				t.Fatalf("%s error = %v", op.name, err)
			}

			parent, err := overlay.Stat(ctx, "/p")
			if err != nil {
				t.Fatalf("Stat(/p) error = %v", err)
			}
			if got := parent.Mode().Perm(); got != 0o750 {
				t.Fatalf("Stat(/p).Mode().Perm() = %o, want 750", got)
			}
			if !parent.ModTime().Equal(lowerTime) {
				t.Fatalf("Stat(/p).ModTime() = %v, want %v", parent.ModTime(), lowerTime)
			}
			dir, err := overlay.Stat(ctx, "/p/d")
			if err != nil {
				t.Fatalf("Stat(/p/d) error = %v", err)
			}
			if ownership, ok := OwnershipFromFileInfo(dir); !ok || ownership.UID != 7 || ownership.GID != 8 {
				t.Fatalf("Stat(/p/d) ownership = %+v (ok=%v), want 7:8", ownership, ok)
			}
		})
	}
}

func overlayWriteFile(ctx context.Context, o *OverlayFS, name string, flag int) error {
	file, err := o.OpenFile(ctx, name, flag, 0)
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte("x")); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
