package fs_test

import (
	"context"
	"errors"
	stdfs "io/fs"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
)

func TestBenchmarkBackendsSymlinkSemantics(t *testing.T) {
	t.Parallel()

	for _, backend := range benchmarkBackends() {
		t.Run(backend.name, func(t *testing.T) {
			fsys := backend.new(t)
			writeFile(t, fsys, "/safe/target.txt", "hello\n")
			if err := fsys.Symlink(context.Background(), "target.txt", "/safe/link.txt"); err != nil {
				t.Fatalf("Symlink() error = %v", err)
			}

			lstatInfo, err := fsys.Lstat(context.Background(), "/safe/link.txt")
			if err != nil {
				t.Fatalf("Lstat() error = %v", err)
			}
			if lstatInfo.Mode()&stdfs.ModeSymlink == 0 {
				t.Fatalf("Lstat().Mode() = %v, want symlink", lstatInfo.Mode())
			}

			target, err := fsys.Readlink(context.Background(), "/safe/link.txt")
			if err != nil {
				t.Fatalf("Readlink() error = %v", err)
			}
			if got, want := target, "target.txt"; got != want {
				t.Fatalf("Readlink() = %q, want %q", got, want)
			}

			realpath, err := fsys.Realpath(context.Background(), "/safe/link.txt")
			if err != nil {
				t.Fatalf("Realpath() error = %v", err)
			}
			if got, want := realpath, "/safe/target.txt"; got != want {
				t.Fatalf("Realpath() = %q, want %q", got, want)
			}

			if got, want := readFile(t, fsys, "/safe/link.txt"), "hello\n"; got != want {
				t.Fatalf("Open(link) = %q, want %q", got, want)
			}
		})
	}
}

func TestBenchmarkBackendsSymlinkLoopFails(t *testing.T) {
	t.Parallel()

	for _, backend := range benchmarkBackends() {
		t.Run(backend.name, func(t *testing.T) {
			fsys := backend.new(t)
			if err := fsys.Symlink(context.Background(), "b", "/a"); err != nil {
				t.Fatalf("Symlink(a) error = %v", err)
			}
			if err := fsys.Symlink(context.Background(), "a", "/b"); err != nil {
				t.Fatalf("Symlink(b) error = %v", err)
			}

			_, err := fsys.Realpath(context.Background(), "/a")
			if err == nil {
				t.Fatal("Realpath() error = nil, want symlink loop")
			}
			if !strings.Contains(err.Error(), "too many levels of symbolic links") {
				t.Fatalf("Realpath() error = %v, want symlink loop message", err)
			}
		})
	}
}

func TestBenchmarkBackendsLazyHardLinksShareProvider(t *testing.T) {
	t.Parallel()

	for _, backend := range benchmarkBackends() {
		t.Run(backend.name, func(t *testing.T) {
			var calls atomic.Int32
			fsys := backend.seeded(t, gbfs.InitialFiles{
				"/lazy.txt": {
					Lazy: func(context.Context) ([]byte, error) {
						calls.Add(1)
						return []byte("lazy\n"), nil
					},
				},
			})

			if err := fsys.Link(context.Background(), "/lazy.txt", "/lazy-link.txt"); err != nil {
				t.Fatalf("Link() error = %v", err)
			}
			if got, want := readFile(t, fsys, "/lazy-link.txt"), "lazy\n"; got != want {
				t.Fatalf("read hard link = %q, want %q", got, want)
			}
			if got, want := calls.Load(), int32(1); got != want {
				t.Fatalf("provider calls = %d, want %d", got, want)
			}
		})
	}
}

func TestBenchmarkBackendsCloneIsolation(t *testing.T) {
	t.Parallel()

	for _, backend := range benchmarkBackends() {
		t.Run(backend.name, func(t *testing.T) {
			base := backend.seeded(t, gbfs.InitialFiles{
				"/data.txt": {Content: []byte("base\n")},
			})
			clone := backend.clone(t, base)

			writeFile(t, clone, "/data.txt", "clone\n")
			writeFile(t, clone, "/new.txt", "new\n")

			if got, want := readFile(t, clone, "/data.txt"), "clone\n"; got != want {
				t.Fatalf("clone /data.txt = %q, want %q", got, want)
			}
			if got, want := readFile(t, base, "/data.txt"), "base\n"; got != want {
				t.Fatalf("base /data.txt = %q, want %q", got, want)
			}
			if _, err := base.Stat(context.Background(), "/new.txt"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("base Stat(/new.txt) error = %v, want not exist", err)
			}
		})
	}
}

func TestBenchmarkBackendsSnapshotAndOverlay(t *testing.T) {
	t.Parallel()

	for _, backend := range benchmarkBackends() {
		t.Run(backend.name, func(t *testing.T) {
			base := backend.seeded(t, gbfs.InitialFiles{
				"/base.txt":       {Content: []byte("base\n")},
				"/shared/old.txt": {Content: []byte("old\n")},
				"/dir/lower.txt":  {Content: []byte("lower\n")},
				"/dir/keep.txt":   {Content: []byte("keep\n")},
			})
			snapshot, err := gbfs.NewSnapshot(context.Background(), base)
			if err != nil {
				t.Fatalf("NewSnapshot() error = %v", err)
			}

			writeFile(t, base, "/base.txt", "after\n")
			if got, want := readFile(t, snapshot, "/base.txt"), "base\n"; got != want {
				t.Fatalf("snapshot /base.txt = %q, want %q", got, want)
			}

			overlay := gbfs.NewOverlay(base)
			if got, want := readFile(t, overlay, "/base.txt"), "after\n"; got != want {
				t.Fatalf("overlay read = %q, want %q", got, want)
			}
			writeFile(t, overlay, "/shared/new.txt", "new\n")
			if _, err := base.Stat(context.Background(), "/shared/new.txt"); !errors.Is(err, stdfs.ErrNotExist) {
				t.Fatalf("base Stat(new.txt) error = %v, want not exist", err)
			}
			if err := overlay.Remove(context.Background(), "/dir/lower.txt", false); err != nil {
				t.Fatalf("Remove() error = %v", err)
			}
			entries := mustReadDir(t, overlay, "/dir")
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if got, want := names, []string{"keep.txt"}; !slices.Equal(got, want) {
				t.Fatalf("ReadDir(/dir) = %v, want %v", got, want)
			}
		})
	}
}
