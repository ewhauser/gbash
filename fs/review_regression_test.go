package fs

import (
	"context"
	"errors"
	"io"
	stdfs "io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLimitFileSizeCapsWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fsys := NewMemory()

	raw, err := fsys.OpenFile(ctx, "/big", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	file := LimitFileSize(raw, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 10)
	if n, err := file.Write([]byte("12345678")); n != 8 || err != nil {
		t.Fatalf("Write() = %d, %v", n, err)
	}
	n, err := file.Write([]byte("abcd"))
	if n != 2 || !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Write() past cap = %d, %v; want 2, EFBIG", n, err)
	}
	if _, ok := file.(fileTruncater); ok {
		t.Fatalf("wrapper exposes Truncate the memory file lacks")
	}
	_ = file.Close()

	info, err := fsys.Stat(ctx, "/big")
	if err != nil || info.Size() != 10 {
		t.Fatalf("Stat() = %v, %v; want size 10", info, err)
	}

	appendFlag := os.O_WRONLY | os.O_APPEND
	raw, err = fsys.OpenFile(ctx, "/big", appendFlag, 0)
	if err != nil {
		t.Fatal(err)
	}
	file = LimitFileSize(raw, appendFlag, 10)
	if n, err := file.Write([]byte("x")); n != 0 || !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("append past cap = %d, %v; want 0, EFBIG", n, err)
	}
	_ = file.Close()

	// Read-only opens are not wrapped.
	raw, err = fsys.Open(ctx, "/big")
	if err != nil {
		t.Fatal(err)
	}
	if got := LimitFileSize(raw, os.O_RDONLY, 10); got != raw {
		t.Fatalf("LimitFileSize(read-only) wrapped the file")
	}
	_ = raw.Close()
}

func TestLimitFileSizeCapsHostTruncate(t *testing.T) {
	t.Parallel()
	host, err := os.Create(t.TempDir() + "/f")
	if err != nil {
		t.Fatal(err)
	}
	file := LimitFileSize(host, os.O_RDWR, 10)
	defer func() { _ = file.Close() }()
	truncater, ok := file.(fileTruncater)
	if !ok {
		t.Fatalf("wrapper hides the host file's Truncate")
	}
	if _, ok := file.(io.Seeker); !ok {
		t.Fatalf("wrapper hides the host file's Seek")
	}
	if err := truncater.Truncate(11); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Truncate() past cap = %v; want EFBIG", err)
	}
	if err := truncater.Truncate(10); err != nil {
		t.Fatalf("Truncate() at cap = %v", err)
	}
}

func TestMemoryFIFOWriteBlocksAtCapacity(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fsys := NewMemory()
	if err := fsys.Mkfifo(ctx, "/p", 0o644); err != nil {
		t.Fatal(err)
	}

	var (
		reader File
		wg     sync.WaitGroup
	)
	wg.Go(func() {
		var err error
		reader, err = fsys.OpenFile(ctx, "/p", os.O_RDONLY, 0)
		if err != nil {
			t.Error(err)
		}
	})
	writer, err := fsys.OpenFile(ctx, "/p", os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if reader == nil {
		t.FailNow()
	}

	payload := make([]byte, 3*memoryFIFOCapacity)
	done := make(chan error, 1)
	go func() {
		_, err := writer.Write(payload)
		_ = writer.Close()
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("Write() returned %v before the reader drained the FIFO", err)
	case <-time.After(50 * time.Millisecond):
	}
	fifo := fsys.nodes["/p"].fifo
	fifo.mu.Lock()
	buffered := len(fifo.buf)
	fifo.mu.Unlock()
	if buffered > memoryFIFOCapacity {
		t.Fatalf("FIFO buffered %d bytes, cap is %d", buffered, memoryFIFOCapacity)
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(payload) {
		t.Fatalf("read %d bytes, want %d", len(got), len(payload))
	}
	if err := <-done; err != nil {
		t.Fatalf("Write() = %v", err)
	}
}

func TestMemorySeedRejectsTypeBits(t *testing.T) {
	t.Parallel()
	for _, mode := range []stdfs.FileMode{stdfs.ModeDir | 0o755, stdfs.ModeSymlink | 0o777, stdfs.ModeSocket | 0o644} {
		fsys := NewMemory()
		err := fsys.seedInitialFiles(InitialFiles{
			"/d":   {Mode: mode},
			"/d/x": {Content: []byte("x")},
		}, time.Now())
		if !errors.Is(err, stdfs.ErrInvalid) {
			t.Fatalf("seed with mode %v = %v; want ErrInvalid", mode, err)
		}
	}

	fsys := NewMemory()
	if err := fsys.seedInitialFiles(InitialFiles{"/p": {Mode: stdfs.ModeNamedPipe | 0o600}}, time.Now()); err != nil {
		t.Fatalf("seed FIFO = %v", err)
	}
	info, err := fsys.Lstat(context.Background(), "/p")
	if err != nil || info.Mode() != stdfs.ModeNamedPipe|0o600 {
		t.Fatalf("Lstat(/p) = %v, %v", info, err)
	}
}

func TestMemoryFileSharedHandleConcurrentReads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fsys := NewMemory()
	if err := seedTestFile(ctx, fsys, "/f", []byte(strings.Repeat("x", 4096))); err != nil {
		t.Fatal(err)
	}
	file, err := fsys.Open(ctx, "/f")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		total int
	)
	for range 8 {
		wg.Go(func() {
			buf := make([]byte, 64)
			for {
				n, err := file.Read(buf)
				mu.Lock()
				total += n
				mu.Unlock()
				if err != nil {
					return
				}
			}
		})
	}
	wg.Wait()
	if total != 4096 {
		t.Fatalf("shared handle read %d bytes in total, want 4096", total)
	}
}

func TestCreateExclusiveRefusesDanglingSymlink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for name, fsys := range map[string]FileSystem{"memory": NewMemory(), "trie": NewTrie()} {
		if err := fsys.Symlink(ctx, "/target", "/link"); err != nil {
			t.Fatal(err)
		}
		_, err := fsys.OpenFile(ctx, "/link", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if !errors.Is(err, stdfs.ErrExist) {
			t.Fatalf("%s: O_EXCL on dangling symlink = %v; want ErrExist", name, err)
		}
		if _, err := fsys.Lstat(ctx, "/target"); !errors.Is(err, stdfs.ErrNotExist) {
			t.Fatalf("%s: O_EXCL created the symlink target: %v", name, err)
		}
	}
}

func TestOverlaySymlinkRefusesExistingLowerEntry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lower := NewMemory()
	if err := seedTestFile(ctx, lower, "/f", []byte("lower")); err != nil {
		t.Fatal(err)
	}
	overlay := NewOverlay(lower)
	err := overlay.Symlink(ctx, "/elsewhere", "/f")
	if !errors.Is(err, stdfs.ErrExist) {
		t.Fatalf("Symlink over lower file = %v; want ErrExist", err)
	}
	info, err := overlay.Lstat(ctx, "/f")
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("Lstat(/f) = %v, %v; want the lower regular file", info, err)
	}
}

type fakeSpecialInfo struct{ mode stdfs.FileMode }

func (i fakeSpecialInfo) Name() string         { return "x" }
func (i fakeSpecialInfo) Size() int64          { return 0 }
func (i fakeSpecialInfo) Mode() stdfs.FileMode { return i.mode }
func (i fakeSpecialInfo) ModTime() time.Time   { return time.Time{} }
func (i fakeSpecialInfo) IsDir() bool          { return false }
func (i fakeSpecialInfo) Sys() any             { return nil }

func TestCloneSpecialFileMasksTypeBits(t *testing.T) {
	t.Parallel()
	dst := NewMemory()
	if err := cloneSpecialFile(dst, "/fifo", fakeSpecialInfo{mode: stdfs.ModeNamedPipe | stdfs.ModeSymlink | stdfs.ModeIrregular | 0o640}); err != nil {
		t.Fatal(err)
	}
	if got := dst.nodes["/fifo"].mode; got != stdfs.ModeNamedPipe|0o640 {
		t.Fatalf("cloned FIFO mode = %v", got)
	}
	if err := cloneSpecialFile(dst, "/sock", fakeSpecialInfo{mode: stdfs.ModeSocket | 0o600}); err != nil {
		t.Fatal(err)
	}
	if got := dst.nodes["/sock"].mode; got != 0o600 {
		t.Fatalf("cloned socket mode = %v; want permission bits only", got)
	}
}

// removeFailFS refuses every Remove so a cross-mount rename cannot finish.
type removeFailFS struct{ *MemoryFS }

func (f removeFailFS) Remove(context.Context, string, bool) error {
	return &os.PathError{Op: "remove", Err: stdfs.ErrPermission}
}

func TestMountableCrossMountRenameRollsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	src := NewMemory()
	if err := seedTestFile(ctx, src, "/f", []byte("data")); err != nil {
		t.Fatal(err)
	}
	m := NewMountable(NewMemory())
	if err := m.Mount("/ro", removeFailFS{src}); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename(ctx, "/ro/f", "/f"); !errors.Is(err, stdfs.ErrPermission) {
		t.Fatalf("Rename() = %v; want ErrPermission", err)
	}
	if _, err := m.Lstat(ctx, "/f"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("failed rename left a copy at /f: %v", err)
	}
	if _, err := m.Lstat(ctx, "/ro/f"); err != nil {
		t.Fatalf("source lost: %v", err)
	}
}

func TestMountableCrossMountRenameMovesFIFO(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := NewMountable(NewMemory())
	if err := m.Mount("/mnt", NewMemory()); err != nil {
		t.Fatal(err)
	}
	if err := m.Mkfifo(ctx, "/mnt/p", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Rename(ctx, "/mnt/p", "/p"); err != nil {
		t.Fatalf("Rename(FIFO) = %v", err)
	}
	info, err := m.Lstat(ctx, "/p")
	if err != nil || info.Mode().Type() != stdfs.ModeNamedPipe {
		t.Fatalf("Lstat(/p) = %v, %v; want a FIFO", info, err)
	}
}

func TestResolvePhysicalAppliesDotDotAfterSymlink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fsys := NewMemory()
	if err := fsys.MkdirAll(ctx, "/a/b", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Symlink(ctx, "/a/b", "/lnk"); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ dir, name, want string }{
		{"/", "/lnk/../data", "/a/data"},
		{"/", "lnk/../data", "/a/data"},
		{"/lnk", "../data", "/a/data"},
		{"/", "/a/b/../data", "/a/data"},
		{"/", "/missing/../data", "/data"},
		{"/", "/lnk/data", "/lnk/data"},
	}
	for _, tc := range cases {
		if got := ResolvePhysical(ctx, fsys, tc.dir, tc.name); got != tc.want {
			t.Errorf("ResolvePhysical(%q, %q) = %q; want %q", tc.dir, tc.name, got, tc.want)
		}
	}
}

func seedTestFile(ctx context.Context, fsys FileSystem, name string, data []byte) error {
	file, err := fsys.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
