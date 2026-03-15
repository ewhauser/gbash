package policy

import (
	"context"
	"os"
	"path"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
)

func FuzzCheckPathReadSymlinkPolicy(f *testing.F) {
	f.Add([]byte{0, 0, 0})
	f.Add([]byte{1, 1, 1})
	f.Add([]byte{2, 2, 2})

	f.Fuzz(func(t *testing.T, raw []byte) {
		cursor := newPolicyFuzzCursor(raw)
		mem := gbfs.NewMemory()
		writePolicyFuzzFile(t, mem, "/safe/target.txt", []byte("hello\n"))
		writePolicyFuzzFile(t, mem, "/denied/secret.txt", []byte("secret\n"))

		linkCases := []string{"target.txt", "/safe/target.txt", "/denied/secret.txt"}
		targetCases := []string{"/safe/target.txt", "/safe/link.txt", "/denied/secret.txt"}
		linkTarget := linkCases[cursor.Intn(len(linkCases))]
		target := targetCases[cursor.Intn(len(targetCases))]
		follow := cursor.Intn(2) == 1

		if err := mem.Symlink(context.Background(), linkTarget, "/safe/link.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}

		pol := NewStatic(&Config{
			ReadRoots:   []string{"/safe"},
			WriteRoots:  []string{"/safe"},
			SymlinkMode: SymlinkMode(map[bool]string{false: string(SymlinkDeny), true: string(SymlinkFollow)}[follow]),
		})

		err := CheckPath(context.Background(), pol, mem, FileActionRead, target)
		expectAllowed := target == "/safe/target.txt"
		if target == "/safe/link.txt" {
			expectAllowed = follow && linkTarget != "/denied/secret.txt"
		}
		if expectAllowed && err != nil {
			t.Fatalf("CheckPath(read, %q) error = %v, want nil", target, err)
		}
		if !expectAllowed && err == nil {
			t.Fatalf("CheckPath(read, %q) unexpectedly allowed linkTarget=%q follow=%v", target, linkTarget, follow)
		}
	})
}

func FuzzCheckPathWriteSymlinkPolicy(f *testing.F) {
	f.Add([]byte{0, 0, 0})
	f.Add([]byte{1, 1, 1})
	f.Add([]byte{2, 2, 2})

	f.Fuzz(func(t *testing.T, raw []byte) {
		cursor := newPolicyFuzzCursor(raw)
		mem := gbfs.NewMemory()
		writePolicyFuzzFile(t, mem, "/safe/target.txt", []byte("hello\n"))
		writePolicyFuzzFile(t, mem, "/denied/secret.txt", []byte("secret\n"))

		linkCases := []string{"target.txt", "/safe/target.txt", "/denied/secret.txt"}
		targetCases := []string{"/safe/target.txt", "/safe/link.txt", "/denied/secret.txt"}
		linkTarget := linkCases[cursor.Intn(len(linkCases))]
		target := targetCases[cursor.Intn(len(targetCases))]
		follow := cursor.Intn(2) == 1

		if err := mem.Symlink(context.Background(), linkTarget, "/safe/link.txt"); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}

		pol := NewStatic(&Config{
			ReadRoots:   []string{"/safe"},
			WriteRoots:  []string{"/safe"},
			SymlinkMode: SymlinkMode(map[bool]string{false: string(SymlinkDeny), true: string(SymlinkFollow)}[follow]),
		})

		err := CheckPath(context.Background(), pol, mem, FileActionWrite, target)
		expectAllowed := target == "/safe/target.txt"
		if target == "/safe/link.txt" {
			expectAllowed = follow && linkTarget != "/denied/secret.txt"
		}
		if expectAllowed && err != nil {
			t.Fatalf("CheckPath(write, %q) error = %v, want nil", target, err)
		}
		if !expectAllowed && err == nil {
			t.Fatalf("CheckPath(write, %q) unexpectedly allowed linkTarget=%q follow=%v", target, linkTarget, follow)
		}
	})
}

type policyFuzzCursor struct {
	data []byte
	idx  int
}

func newPolicyFuzzCursor(data []byte) *policyFuzzCursor {
	return &policyFuzzCursor{data: data}
}

func (c *policyFuzzCursor) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	if len(c.data) == 0 {
		c.idx++
		return c.idx % n
	}
	value := int(c.data[c.idx%len(c.data)])
	c.idx++
	return value % n
}

func writePolicyFuzzFile(t *testing.T, fsys gbfs.FileSystem, name string, data []byte) {
	t.Helper()

	if err := fsys.MkdirAll(context.Background(), path.Dir(name), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path.Dir(name), err)
	}
	file, err := fsys.OpenFile(context.Background(), name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", name, err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(data); err != nil {
		t.Fatalf("Write(%q) error = %v", name, err)
	}
}
