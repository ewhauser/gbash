package builtins

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type tailTestFileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (i tailTestFileInfo) Name() string       { return i.name }
func (i tailTestFileInfo) Size() int64        { return i.size }
func (i tailTestFileInfo) Mode() os.FileMode  { return i.mode }
func (i tailTestFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (i tailTestFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i tailTestFileInfo) Sys() any           { return nil }

func TestTailSameFileInfoDetectsRenameOverExistingFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	aPath := filepath.Join(tmp, "a")
	bPath := filepath.Join(tmp, "b")
	if err := os.WriteFile(aPath, []byte("a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(a) error = %v", err)
	}
	if err := os.WriteFile(bPath, []byte("b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(b) error = %v", err)
	}

	aInfoBefore, err := os.Stat(aPath)
	if err != nil {
		t.Fatalf("Stat(a before) error = %v", err)
	}
	bInfoBefore, err := os.Stat(bPath)
	if err != nil {
		t.Fatalf("Stat(b before) error = %v", err)
	}

	if err := os.Rename(aPath, bPath); err != nil {
		t.Fatalf("Rename(a, b) error = %v", err)
	}
	bInfoAfter, err := os.Stat(bPath)
	if err != nil {
		t.Fatalf("Stat(b after) error = %v", err)
	}

	if same, known := tailSameFileInfo(bInfoBefore, bInfoAfter); !known || same {
		t.Fatalf("tailSameFileInfo(old b, new b) = (%v, %v), want (false, true)", same, known)
	}
	if same, known := tailSameFileInfo(aInfoBefore, bInfoAfter); !known || !same {
		t.Fatalf("tailSameFileInfo(old a, new b) = (%v, %v), want (true, true)", same, known)
	}
}

func TestTailSameFileInfoReturnsUnknownWithoutComparableIdentity(t *testing.T) {
	t.Parallel()
	info := tailTestFileInfo{name: "virtual", mode: 0o644}
	if same, known := tailSameFileInfo(info, info); known || same {
		t.Fatalf("tailSameFileInfo(virtual, virtual) = (%v, %v), want (false, false)", same, known)
	}
}

func TestParseTailSleepIntervalClampsToTickerRange(t *testing.T) {
	t.Parallel()

	cases := map[string]time.Duration{
		"0":      time.Millisecond,
		"0.0001": time.Millisecond,
		"1.5":    1500 * time.Millisecond,
		"1e300":  time.Duration(1<<63 - 1),
		"inf":    time.Duration(1<<63 - 1),
	}
	for raw, want := range cases {
		got, err := parseTailSleepInterval(raw)
		if err != nil || got != want {
			t.Fatalf("parseTailSleepInterval(%q) = %v, %v; want %v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"-1", "nan", "x"} {
		if _, err := parseTailSleepInterval(raw); err == nil {
			t.Fatalf("parseTailSleepInterval(%q) error = nil, want invalid interval", raw)
		}
	}
}
