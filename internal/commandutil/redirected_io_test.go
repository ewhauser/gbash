package commandutil

import (
	"io"
	stdfs "io/fs"
	"testing"
	"time"
)

type redirectedFileInfo struct{}

func (redirectedFileInfo) Name() string         { return "redirected" }
func (redirectedFileInfo) Size() int64          { return 1 }
func (redirectedFileInfo) Mode() stdfs.FileMode { return 0 }
func (redirectedFileInfo) ModTime() time.Time   { return time.Time{} }
func (redirectedFileInfo) IsDir() bool          { return false }
func (redirectedFileInfo) Sys() any             { return nil }

type redirectedFileWithDeadline struct {
	deadline time.Time
}

func (f *redirectedFileWithDeadline) Read(p []byte) (int, error)  { return 0, io.EOF }
func (f *redirectedFileWithDeadline) Write(p []byte) (int, error) { return len(p), nil }
func (f *redirectedFileWithDeadline) Close() error                { return nil }
func (f *redirectedFileWithDeadline) Stat() (stdfs.FileInfo, error) {
	return redirectedFileInfo{}, nil
}
func (f *redirectedFileWithDeadline) SetReadDeadline(t time.Time) error {
	f.deadline = t
	return nil
}

func TestWrapRedirectedFileDelegatesReadDeadline(t *testing.T) {
	t.Parallel()

	base := &redirectedFileWithDeadline{}
	wrapped, ok := WrapRedirectedFile(base, "/tmp/in", 0).(interface {
		SetReadDeadline(time.Time) error
	})
	if !ok {
		t.Fatalf("wrapped file is missing SetReadDeadline")
	}

	deadline := time.Unix(123, 456)
	if err := wrapped.SetReadDeadline(deadline); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if got, want := base.deadline, deadline; !got.Equal(want) {
		t.Fatalf("deadline = %v, want %v", got, want)
	}
}
