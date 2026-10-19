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

// newSymlinkOverlay returns an overlay whose lower layer holds /w/real/f,
// /w/link -> real, and /w/flink -> real/f.
func newSymlinkOverlay(t *testing.T) (*OverlayFS, *MemoryFS) {
	t.Helper()
	ctx := context.Background()
	lower := seededMemory(t, map[string]string{
		"/w/real/f": "orig",
	})
	if err := lower.Symlink(ctx, "real", "/w/link"); err != nil {
		t.Fatalf("Symlink(link) error = %v", err)
	}
	if err := lower.Symlink(ctx, "real/f", "/w/flink"); err != nil {
		t.Fatalf("Symlink(flink) error = %v", err)
	}
	return NewOverlay(lower), lower
}

func requireOverlaySymlink(t *testing.T, fsys FileSystem, name, wantTarget string) {
	t.Helper()
	info, err := fsys.Lstat(context.Background(), name)
	if err != nil {
		t.Fatalf("Lstat(%q) error = %v", name, err)
	}
	if info.Mode()&stdfs.ModeSymlink == 0 {
		t.Fatalf("Lstat(%q).Mode() = %v, want symlink", name, info.Mode())
	}
	target, err := fsys.Readlink(context.Background(), name)
	if err != nil {
		t.Fatalf("Readlink(%q) error = %v", name, err)
	}
	if target != wantTarget {
		t.Fatalf("Readlink(%q) = %q, want %q", name, target, wantTarget)
	}
}

func TestOverlayFSStatFollowsLowerSymlinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, _ := newSymlinkOverlay(t)

	info, err := overlay.Stat(ctx, "/w/link")
	if err != nil {
		t.Fatalf("Stat(link) error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("Stat(link).Mode() = %v, want directory", info.Mode())
	}
	info, err = overlay.Stat(ctx, "/w/flink")
	if err != nil {
		t.Fatalf("Stat(flink) error = %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("Stat(flink).Mode() = %v, want regular file", info.Mode())
	}
	requireOverlaySymlink(t, overlay, "/w/link", "real")
	requireOverlaySymlink(t, overlay, "/w/flink", "real/f")
}

func TestOverlayFSResolvesSymlinksAcrossLayers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, lower := newSymlinkOverlay(t)

	// Upper-layer link pointing at a lower-only directory.
	if err := overlay.Symlink(ctx, "real", "/w/ulink"); err != nil {
		t.Fatalf("Symlink(ulink) error = %v", err)
	}
	// Lower-layer link pointing at a directory that only exists in the upper layer.
	if err := lower.Symlink(ctx, "fresh", "/w/tofresh"); err != nil {
		t.Fatalf("Symlink(tofresh) error = %v", err)
	}
	writeTestFile(t, overlay, "/w/fresh/x", "x")

	for _, name := range []string{"/w/ulink", "/w/tofresh"} {
		info, err := overlay.Stat(ctx, name)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("Stat(%q).Mode() = %v, want directory", name, info.Mode())
		}
	}
	if got, want := readTestFile(t, overlay, "/w/ulink/f"), "orig"; got != want {
		t.Fatalf("read ulink/f = %q, want %q", got, want)
	}
	if got, want := readTestFile(t, overlay, "/w/tofresh/x"), "x"; got != want {
		t.Fatalf("read tofresh/x = %q, want %q", got, want)
	}
	if info, err := overlay.Lstat(ctx, "/w/tofresh/x"); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("Lstat(tofresh/x) = %v, %v; want regular file", info, err)
	}

	for name, want := range map[string]string{
		"/w/ulink":     "/w/real",
		"/w/ulink/f":   "/w/real/f",
		"/w/tofresh":   "/w/fresh",
		"/w/tofresh/x": "/w/fresh/x",
	} {
		got, err := overlay.Realpath(ctx, name)
		if err != nil {
			t.Fatalf("Realpath(%q) error = %v", name, err)
		}
		if got != want {
			t.Fatalf("Realpath(%q) = %q, want %q", name, got, want)
		}
	}

	entries, err := overlay.ReadDir(ctx, "/w/link")
	if err != nil {
		t.Fatalf("ReadDir(link) error = %v", err)
	}
	if got := dirEntryNames(entries); !slices.Equal(got, []string{"f"}) {
		t.Fatalf("ReadDir(link) = %v, want [f]", got)
	}
	entries, err = overlay.ReadDir(ctx, "/w/tofresh")
	if err != nil {
		t.Fatalf("ReadDir(tofresh) error = %v", err)
	}
	if got := dirEntryNames(entries); !slices.Equal(got, []string{"x"}) {
		t.Fatalf("ReadDir(tofresh) = %v, want [x]", got)
	}

	if err := overlay.Chdir("/w/link"); err != nil {
		t.Fatalf("Chdir(link) error = %v", err)
	}
	if got, want := readTestFile(t, overlay, "f"), "orig"; got != want {
		t.Fatalf("read f after Chdir(link) = %q, want %q", got, want)
	}
}

func TestOverlayFSMkdirAllFollowsSymlinkedComponents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, lower := newSymlinkOverlay(t)

	if err := overlay.MkdirAll(ctx, "/w/link/sub/deeper", 0o755); err != nil {
		t.Fatalf("MkdirAll(link/sub/deeper) error = %v", err)
	}
	info, err := overlay.Stat(ctx, "/w/real/sub/deeper")
	if err != nil {
		t.Fatalf("Stat(real/sub/deeper) error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("Stat(real/sub/deeper).Mode() = %v, want directory", info.Mode())
	}
	requireOverlaySymlink(t, overlay, "/w/link", "real")
	if _, err := lower.Stat(ctx, "/w/real/sub"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("lower Stat(real/sub) error = %v, want not exist", err)
	}

	// Existing components through a symlink are accepted without error.
	if err := overlay.MkdirAll(ctx, "/w/link", 0o755); err != nil {
		t.Fatalf("MkdirAll(link) error = %v", err)
	}
	// A symlink to a file is still not a directory.
	if err := overlay.MkdirAll(ctx, "/w/flink/sub", 0o755); err == nil {
		t.Fatalf("MkdirAll(flink/sub) error = nil, want not a directory")
	}
}

func TestOverlayFSWriteThroughLowerFileSymlinkWritesTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, lower := newSymlinkOverlay(t)

	writeTestFile(t, overlay, "/w/flink", "new")

	if got, want := readTestFile(t, overlay, "/w/real/f"), "new"; got != want {
		t.Fatalf("read real/f = %q, want %q", got, want)
	}
	if got, want := readTestFile(t, overlay, "/w/flink"), "new"; got != want {
		t.Fatalf("read flink = %q, want %q", got, want)
	}
	requireOverlaySymlink(t, overlay, "/w/flink", "real/f")
	info, err := overlay.Stat(ctx, "/w/real/f")
	if err != nil {
		t.Fatalf("Stat(real/f) error = %v", err)
	}
	if got, want := info.Mode().Perm(), stdfs.FileMode(0o644); got != want {
		t.Fatalf("Stat(real/f).Mode().Perm() = %v, want %v", got, want)
	}
	if got, want := readTestFile(t, lower, "/w/real/f"), "orig"; got != want {
		t.Fatalf("lower real/f = %q, want %q", got, want)
	}
}

func TestOverlayFSCreateThroughSymlinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, lower := newSymlinkOverlay(t)
	if err := lower.Symlink(ctx, "real/missing", "/w/dangling"); err != nil {
		t.Fatalf("Symlink(dangling) error = %v", err)
	}

	writeTestFile(t, overlay, "/w/link/new", "created")
	if got, want := readTestFile(t, overlay, "/w/real/new"), "created"; got != want {
		t.Fatalf("read real/new = %q, want %q", got, want)
	}
	requireOverlaySymlink(t, overlay, "/w/link", "real")

	writeTestFile(t, overlay, "/w/dangling", "via-dangling")
	if got, want := readTestFile(t, overlay, "/w/real/missing"), "via-dangling"; got != want {
		t.Fatalf("read real/missing = %q, want %q", got, want)
	}
	requireOverlaySymlink(t, overlay, "/w/dangling", "real/missing")

	if err := overlay.Symlink(ctx, "real/upper-missing", "/w/udangling"); err != nil {
		t.Fatalf("Symlink(udangling) error = %v", err)
	}
	for _, name := range []string{"/w/flink", "/w/dangling", "/w/udangling"} {
		if _, err := overlay.OpenFile(ctx, name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); !errors.Is(err, stdfs.ErrExist) {
			t.Fatalf("OpenFile(%q, O_EXCL) error = %v, want exist", name, err)
		}
	}
	if _, err := overlay.Lstat(ctx, "/w/real/upper-missing"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Lstat(real/upper-missing) error = %v, want not exist", err)
	}
}

func TestOverlayFSMetadataChangesFollowSymlinks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, _ := newSymlinkOverlay(t)

	if err := overlay.Chmod(ctx, "/w/flink", 0o600); err != nil {
		t.Fatalf("Chmod(flink) error = %v", err)
	}
	info, err := overlay.Stat(ctx, "/w/real/f")
	if err != nil {
		t.Fatalf("Stat(real/f) error = %v", err)
	}
	if got, want := info.Mode().Perm(), stdfs.FileMode(0o600); got != want {
		t.Fatalf("Stat(real/f).Mode().Perm() = %v, want %v", got, want)
	}
	requireOverlaySymlink(t, overlay, "/w/flink", "real/f")

	mtime := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := overlay.Chtimes(ctx, "/w/flink", mtime, mtime); err != nil {
		t.Fatalf("Chtimes(flink) error = %v", err)
	}
	if info, err := overlay.Stat(ctx, "/w/real/f"); err != nil || !info.ModTime().Equal(mtime) {
		t.Fatalf("Stat(real/f) = %v, %v; want mtime %v", info, err, mtime)
	}

	if err := overlay.Chown(ctx, "/w/flink", 1234, 5678, true); err != nil {
		t.Fatalf("Chown(flink, follow) error = %v", err)
	}
	info, err = overlay.Stat(ctx, "/w/real/f")
	if err != nil {
		t.Fatalf("Stat(real/f) error = %v", err)
	}
	if owner, ok := OwnershipFromFileInfo(info); !ok || owner.UID != 1234 || owner.GID != 5678 {
		t.Fatalf("Stat(real/f) ownership = %+v, %v; want 1234:5678", owner, ok)
	}
	requireOverlaySymlink(t, overlay, "/w/flink", "real/f")
	if got, want := readTestFile(t, overlay, "/w/flink"), "orig"; got != want {
		t.Fatalf("read flink = %q, want %q", got, want)
	}

	// Metadata changes through a symlinked parent land on the target.
	if err := overlay.Chmod(ctx, "/w/link/f", 0o640); err != nil {
		t.Fatalf("Chmod(link/f) error = %v", err)
	}
	if info, err := overlay.Stat(ctx, "/w/real/f"); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("Stat(real/f) = %v, %v; want perm 0640", info, err)
	}
	requireOverlaySymlink(t, overlay, "/w/link", "real")
}

func TestOverlayFSRemoveThroughSymlinkedParent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	overlay, _ := newSymlinkOverlay(t)

	if err := overlay.Remove(ctx, "/w/link/f", false); err != nil {
		t.Fatalf("Remove(link/f) error = %v", err)
	}
	if _, err := overlay.Lstat(ctx, "/w/real/f"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("Lstat(real/f) error = %v, want not exist", err)
	}
	requireOverlaySymlink(t, overlay, "/w/link", "real")

	// Removing the link itself leaves the target alone.
	if err := overlay.Remove(ctx, "/w/link", false); err != nil {
		t.Fatalf("Remove(link) error = %v", err)
	}
	if info, err := overlay.Stat(ctx, "/w/real"); err != nil || !info.IsDir() {
		t.Fatalf("Stat(real) = %v, %v; want directory", info, err)
	}
}

func dirEntryNames(entries []stdfs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
