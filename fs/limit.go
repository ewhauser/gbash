package fs

import (
	"errors"
	"io"
	stdfs "io/fs"
	"os"
	"syscall"
)

// ErrFileTooLarge is returned when a write or truncate would grow a regular
// file past the size cap given to [LimitFileSize]. It matches EFBIG, whose
// message is "file too large".
var ErrFileTooLarge error = syscall.EFBIG

// LimitFileSize wraps file so writes and truncates cannot grow it past
// maxBytes. Writes that would cross the cap store the bytes that fit and then
// fail with [ErrFileTooLarge], as a host write does under RLIMIT_FSIZE.
//
// Only regular files opened with write intent are wrapped. Other files, and
// any file when maxBytes is not positive, are returned unchanged.
func LimitFileSize(file File, flag int, maxBytes int64) File {
	if file == nil || maxBytes <= 0 || !hasWriteIntent(flag) {
		return file
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return file
	}
	limited := &sizeLimitedFile{File: file, flag: flag, max: maxBytes}
	seeker, canSeek := file.(io.Seeker)
	if canSeek {
		if pos, err := seeker.Seek(0, io.SeekCurrent); err == nil {
			limited.offset = pos
		}
	}
	_, canTruncate := file.(fileTruncater)
	// Expose Seek and Truncate only when the wrapped file has them, so
	// callers that probe for them keep their fallbacks.
	switch {
	case canSeek && canTruncate:
		return &sizeLimitedSeekTruncateFile{limited}
	case canSeek:
		return &sizeLimitedSeekFile{limited}
	case canTruncate:
		return &sizeLimitedTruncateFile{limited}
	default:
		return limited
	}
}

type fileTruncater interface {
	Truncate(size int64) error
}

type sizeLimitedSeekFile struct{ *sizeLimitedFile }

func (f *sizeLimitedSeekFile) Seek(offset int64, whence int) (int64, error) {
	return f.seek(offset, whence)
}

type sizeLimitedTruncateFile struct{ *sizeLimitedFile }

func (f *sizeLimitedTruncateFile) Truncate(size int64) error {
	return f.truncate(size)
}

type sizeLimitedSeekTruncateFile struct{ *sizeLimitedFile }

func (f *sizeLimitedSeekTruncateFile) Seek(offset int64, whence int) (int64, error) {
	return f.seek(offset, whence)
}

func (f *sizeLimitedSeekTruncateFile) Truncate(size int64) error {
	return f.truncate(size)
}

type sizeLimitedFile struct {
	File
	flag   int
	max    int64
	offset int64
}

func (f *sizeLimitedFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *sizeLimitedFile) Write(p []byte) (int, error) {
	if f.flag&os.O_APPEND != 0 {
		if info, err := f.Stat(); err == nil {
			f.offset = info.Size()
		}
	}
	allowed := max(f.max-f.offset, 0)
	if int64(len(p)) <= allowed {
		n, err := f.File.Write(p)
		f.offset += int64(n)
		return n, err
	}
	n := 0
	if allowed > 0 {
		var err error
		n, err = f.File.Write(p[:allowed])
		f.offset += int64(n)
		if err != nil {
			return n, err
		}
	}
	return n, f.tooLarge("write")
}

func (f *sizeLimitedFile) seek(offset int64, whence int) (int64, error) {
	seeker, ok := f.File.(io.Seeker)
	if !ok {
		return 0, &os.PathError{Op: "seek", Path: f.name(), Err: errors.ErrUnsupported}
	}
	pos, err := seeker.Seek(offset, whence)
	if err == nil {
		f.offset = pos
	}
	return pos, err
}

func (f *sizeLimitedFile) truncate(size int64) error {
	if size > f.max {
		return f.tooLarge("truncate")
	}
	truncater, ok := f.File.(fileTruncater)
	if !ok {
		return &os.PathError{Op: "truncate", Path: f.name(), Err: errors.ErrUnsupported}
	}
	return truncater.Truncate(size)
}

// Unwrap returns the wrapped file.
func (f *sizeLimitedFile) Unwrap() File {
	return f.File
}

func (f *sizeLimitedFile) name() string {
	if info, err := f.Stat(); err == nil {
		return info.Name()
	}
	return ""
}

func (f *sizeLimitedFile) tooLarge(op string) error {
	return &os.PathError{Op: op, Path: f.name(), Err: ErrFileTooLarge}
}

var _ stdfs.File = (*sizeLimitedFile)(nil)
