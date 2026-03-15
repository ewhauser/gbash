package fs_test

import (
	"context"
	"io"
	stdfs "io/fs"
	"os"
	"path"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
)

type filesystemBackend struct {
	name   string
	new    func(t testing.TB) gbfs.FileSystem
	seeded func(t testing.TB, files gbfs.InitialFiles) gbfs.FileSystem
	clone  func(t testing.TB, fsys gbfs.FileSystem) gbfs.FileSystem
}

func benchmarkBackends() []filesystemBackend {
	return []filesystemBackend{
		{
			name: "memory",
			new: func(testing.TB) gbfs.FileSystem {
				return gbfs.NewMemory()
			},
			seeded: func(t testing.TB, files gbfs.InitialFiles) gbfs.FileSystem {
				t.Helper()
				fsys, err := gbfs.SeededMemory(files).New(context.Background())
				if err != nil {
					t.Fatalf("SeededMemory.New() error = %v", err)
				}
				return fsys
			},
			clone: func(t testing.TB, fsys gbfs.FileSystem) gbfs.FileSystem {
				t.Helper()
				base, ok := fsys.(*gbfs.MemoryFS)
				if !ok {
					t.Fatalf("clone backend %T, want *fs.MemoryFS", fsys)
				}
				return base.Clone()
			},
		},
		{
			name: "trie",
			new: func(testing.TB) gbfs.FileSystem {
				return gbfs.NewTrie()
			},
			seeded: func(t testing.TB, files gbfs.InitialFiles) gbfs.FileSystem {
				t.Helper()
				fsys, err := gbfs.SeededTrie(files).New(context.Background())
				if err != nil {
					t.Fatalf("SeededTrie.New() error = %v", err)
				}
				return fsys
			},
			clone: func(t testing.TB, fsys gbfs.FileSystem) gbfs.FileSystem {
				t.Helper()
				base, ok := fsys.(*gbfs.TrieFS)
				if !ok {
					t.Fatalf("clone backend %T, want *fs.TrieFS", fsys)
				}
				return base.Clone()
			},
		},
	}
}

func writeFile(t testing.TB, fsys gbfs.FileSystem, name, contents string) {
	t.Helper()

	if err := fsys.MkdirAll(context.Background(), path.Dir(name), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path.Dir(name), err)
	}
	file, err := fsys.OpenFile(context.Background(), name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", name, err)
	}
	if _, err := io.WriteString(file, contents); err != nil {
		_ = file.Close()
		t.Fatalf("WriteString(%q) error = %v", name, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(%q) error = %v", name, err)
	}
}

func readFile(t testing.TB, fsys gbfs.FileSystem, name string) string {
	t.Helper()

	file, err := fsys.Open(context.Background(), name)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", name, err)
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll(%q) error = %v", name, err)
	}
	return string(data)
}

func mustReadDir(t testing.TB, fsys gbfs.FileSystem, name string) []stdfs.DirEntry {
	t.Helper()

	entries, err := fsys.ReadDir(context.Background(), name)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", name, err)
	}
	return entries
}
