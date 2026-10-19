package fs

import (
	"context"
	"errors"
	stdfs "io/fs"
	"os"
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
