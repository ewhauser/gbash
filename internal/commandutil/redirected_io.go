package commandutil

import (
	"io"
	stdfs "io/fs"
	"os"
	"time"

	gbfs "github.com/ewhauser/gbash/fs"
)

type RedirectMetadata interface {
	RedirectPath() string
	RedirectFlags() int
	RedirectOffset() int64
}

type redirectedFile struct {
	file   gbfs.File
	path   string
	flag   int
	offset int64
}

func WrapRedirectedFile(file gbfs.File, path string, flag int) io.ReadWriteCloser {
	if file == nil {
		return nil
	}
	offset := int64(0)
	if seeker, ok := file.(interface {
		Seek(offset int64, whence int) (int64, error)
	}); ok {
		if position, err := seeker.Seek(0, io.SeekCurrent); err == nil && position >= 0 {
			offset = position
		}
	}
	return &redirectedFile{
		file:   file,
		path:   path,
		flag:   flag,
		offset: offset,
	}
}

func (f *redirectedFile) Read(p []byte) (int, error) {
	n, err := f.file.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *redirectedFile) Write(p []byte) (int, error) {
	if f.flag&os.O_APPEND != 0 {
		if info, err := f.file.Stat(); err == nil {
			f.offset = info.Size()
		}
	}
	n, err := f.file.Write(p)
	f.offset += int64(n)
	return n, err
}

func (f *redirectedFile) Close() error {
	return f.file.Close()
}

func (f *redirectedFile) Stat() (stdfs.FileInfo, error) {
	return f.file.Stat()
}

func (f *redirectedFile) Seek(offset int64, whence int) (int64, error) {
	seeker, ok := f.file.(interface {
		Seek(offset int64, whence int) (int64, error)
	})
	if !ok {
		return 0, stdfs.ErrInvalid
	}
	position, err := seeker.Seek(offset, whence)
	if err == nil && position >= 0 {
		f.offset = position
	}
	return position, err
}

func (f *redirectedFile) Fd() uintptr {
	file, ok := f.file.(interface{ Fd() uintptr })
	if !ok {
		return 0
	}
	return file.Fd()
}

func (f *redirectedFile) SetReadDeadline(time.Time) error {
	return nil
}

func (f *redirectedFile) RedirectPath() string {
	return f.path
}

func (f *redirectedFile) RedirectFlags() int {
	return f.flag
}

func (f *redirectedFile) RedirectOffset() int64 {
	return f.offset
}

var _ gbfs.File = (*redirectedFile)(nil)
var _ RedirectMetadata = (*redirectedFile)(nil)
