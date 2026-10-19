package fs

import (
	"context"
	"io"
	"os"
	"testing"
)

func handleBindingBackends() map[string]func() FileSystem {
	return map[string]func() FileSystem{
		"MemoryFS":  func() FileSystem { return NewMemory() },
		"TrieFS":    func() FileSystem { return NewTrie() },
		"OverlayFS": func() FileSystem { return NewOverlay(NewMemory()) },
	}
}

func TestOpenHandleFollowsRenamedFile(t *testing.T) {
	t.Parallel()
	for name, newFS := range handleBindingBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fsys := newFS()
			writer, err := fsys.OpenFile(ctx, "/a", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				t.Fatalf("OpenFile(/a) error = %v", err)
			}
			defer func() { _ = writer.Close() }()
			if err := fsys.Rename(ctx, "/a", "/b"); err != nil {
				t.Fatalf("Rename(/a, /b) error = %v", err)
			}
			if _, err := io.WriteString(writer, "hi\n"); err != nil {
				t.Fatalf("Write() after rename error = %v", err)
			}
			if got := readTestFile(t, fsys, "/b"); got != "hi\n" {
				t.Fatalf("read /b = %q, want %q", got, "hi\n")
			}
			info, err := writer.Stat()
			if err != nil {
				t.Fatalf("Stat() after rename error = %v", err)
			}
			if info.Size() != 3 {
				t.Fatalf("Stat().Size() = %d, want 3", info.Size())
			}
		})
	}
}

func TestOpenHandleKeepsUnlinkedFileContent(t *testing.T) {
	t.Parallel()
	for name, newFS := range handleBindingBackends() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fsys := newFS()
			writeTestFile(t, fsys, "/c", "old\n")
			reader, err := fsys.Open(ctx, "/c")
			if err != nil {
				t.Fatalf("Open(/c) error = %v", err)
			}
			defer func() { _ = reader.Close() }()
			if err := fsys.Remove(ctx, "/c", false); err != nil {
				t.Fatalf("Remove(/c) error = %v", err)
			}
			writeTestFile(t, fsys, "/c", "new\n")

			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if got := string(data); got != "old\n" {
				t.Fatalf("read through old handle = %q, want %q", got, "old\n")
			}
		})
	}
}
