//go:build !windows && !js

package fs

import (
	"context"
	"errors"
	"io"
	stdfs "io/fs"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// hostFIFOPollInterval bounds how long a host FIFO operation waits between
// context checks.
const hostFIFOPollInterval = 20 * time.Millisecond

// openHostFile opens rel under root without ever blocking in the open syscall.
//
// The file is opened with O_NONBLOCK so a host FIFO without a peer, or a device
// that blocks on open, cannot hang the caller past ctx. Regular files and
// directories are returned as plain *os.File values. Named pipes are wrapped in
// a handle whose reads and writes poll and give up once ctx is done. Devices,
// sockets, and other special files are refused.
func openHostFile(ctx context.Context, root *os.Root, rel string, flag int, perm stdfs.FileMode) (File, stdfs.FileInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Refuse devices and sockets before open(2) can run a driver's open hook.
	// The fstat below still catches a node swapped in after this check.
	if info, err := root.Lstat(rel); err == nil && !hostOpenableMode(info.Mode()) {
		return nil, nil, stdfs.ErrPermission
	}
	file, err := openHostNonblock(ctx, root, rel, flag, perm)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	switch mode := info.Mode(); {
	case !hostOpenableMode(mode):
		_ = file.Close()
		return nil, nil, stdfs.ErrPermission
	case mode.IsRegular(), mode.IsDir():
		if err := setHostFileBlocking(file); err != nil {
			_ = file.Close()
			return nil, nil, err
		}
		return file, info, nil
	default:
		fifo, err := newHostFIFOFile(ctx, file, flag)
		if err != nil {
			_ = file.Close()
			return nil, nil, err
		}
		return fifo, info, nil
	}
}

// hostOpenableMode reports whether a host node of this type may be opened:
// regular files, directories, and named pipes, but no devices or sockets.
func hostOpenableMode(mode stdfs.FileMode) bool {
	return mode.IsRegular() || mode.IsDir() || mode.Type() == stdfs.ModeNamedPipe
}

// openHostNonblock opens rel with O_NONBLOCK. Opening a FIFO for writing
// without a reader fails with ENXIO in that mode, so it retries until a reader
// appears or ctx is done, which mirrors the blocking open(2) without ignoring
// cancellation.
func openHostNonblock(ctx context.Context, root *os.Root, rel string, flag int, perm stdfs.FileMode) (*os.File, error) {
	for {
		file, err := root.OpenFile(rel, flag|syscall.O_NONBLOCK, perm)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, syscall.ENXIO) {
			return nil, err
		}
		if info, statErr := root.Lstat(rel); statErr != nil || info.Mode()&stdfs.ModeNamedPipe == 0 {
			return nil, err
		}
		if err := sleepCtx(ctx, hostFIFOPollInterval); err != nil {
			return nil, err
		}
	}
}

func setHostFileBlocking(file *os.File) error {
	rc, err := file.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	if err := rc.Control(func(fd uintptr) {
		opErr = unix.SetNonblock(int(fd), false)
	}); err != nil {
		return err
	}
	return opErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// hostFIFOFile is a host named pipe opened in non-blocking mode. Reads and
// writes retry on EAGAIN with a bounded poll so they observe ctx.
type hostFIFOFile struct {
	file *os.File
	rc   syscall.RawConn
	ctx  context.Context
	flag int

	// sawWriter records that a writer has been attached at some point. A
	// non-blocking read returns 0 both at end of stream and before any writer
	// has opened the FIFO; only the former is EOF, matching a blocking open.
	sawWriter bool
}

func newHostFIFOFile(ctx context.Context, file *os.File, flag int) (*hostFIFOFile, error) {
	rc, err := file.SyscallConn()
	if err != nil {
		return nil, err
	}
	return &hostFIFOFile{file: file, rc: rc, ctx: ctx, flag: flag}, nil
}

func (f *hostFIFOFile) Read(p []byte) (int, error) {
	if !canRead(f.flag) {
		return 0, &os.PathError{Op: "read", Path: f.file.Name(), Err: stdfs.ErrPermission}
	}
	if len(p) == 0 {
		return 0, nil
	}
	for {
		var (
			n     int
			opErr error
		)
		if err := f.rc.Control(func(fd uintptr) {
			n, opErr = unix.Read(int(fd), p)
		}); err != nil {
			return 0, err
		}
		switch {
		case n > 0:
			f.sawWriter = true
			return n, nil
		case opErr == nil:
			if f.sawWriter {
				return 0, io.EOF
			}
		case errors.Is(opErr, unix.EINTR):
			continue
		case errors.Is(opErr, unix.EAGAIN):
			// EAGAIN means a writer is attached but has not written yet.
			f.sawWriter = true
		default:
			return 0, &os.PathError{Op: "read", Path: f.file.Name(), Err: opErr}
		}
		hup, err := f.wait(unix.POLLIN)
		if err != nil {
			return 0, &os.PathError{Op: "read", Path: f.file.Name(), Err: err}
		}
		if hup {
			// A writer attached and detached between polls.
			f.sawWriter = true
		}
	}
}

func (f *hostFIFOFile) Write(p []byte) (int, error) {
	if !canWrite(f.flag) {
		return 0, &os.PathError{Op: "write", Path: f.file.Name(), Err: stdfs.ErrPermission}
	}
	written := 0
	for written < len(p) {
		var (
			n     int
			opErr error
		)
		if err := f.rc.Control(func(fd uintptr) {
			n, opErr = unix.Write(int(fd), p[written:])
		}); err != nil {
			return written, err
		}
		if n > 0 {
			written += n
			continue
		}
		switch {
		case opErr == nil, errors.Is(opErr, unix.EINTR):
			continue
		case errors.Is(opErr, unix.EAGAIN):
		default:
			return written, &os.PathError{Op: "write", Path: f.file.Name(), Err: opErr}
		}
		if _, err := f.wait(unix.POLLOUT); err != nil {
			return written, &os.PathError{Op: "write", Path: f.file.Name(), Err: err}
		}
	}
	return written, nil
}

// wait polls the FIFO for events for at most hostFIFOPollInterval and reports
// whether the peer hung up. It returns ctx's error once ctx is done.
func (f *hostFIFOFile) wait(events int16) (bool, error) {
	if err := f.ctx.Err(); err != nil {
		return false, err
	}
	var (
		revents int16
		opErr   error
	)
	if err := f.rc.Control(func(fd uintptr) {
		fds := []unix.PollFd{{Fd: int32(fd), Events: events}}
		_, opErr = unix.Poll(fds, int(hostFIFOPollInterval/time.Millisecond))
		revents = fds[0].Revents
	}); err != nil {
		return false, err
	}
	if opErr != nil && !errors.Is(opErr, unix.EINTR) {
		return false, opErr
	}
	if err := f.ctx.Err(); err != nil {
		return false, err
	}
	return revents&unix.POLLHUP != 0, nil
}

func (f *hostFIFOFile) Close() error {
	return f.file.Close()
}

func (f *hostFIFOFile) Stat() (stdfs.FileInfo, error) {
	return f.file.Stat()
}

var _ File = (*hostFIFOFile)(nil)
