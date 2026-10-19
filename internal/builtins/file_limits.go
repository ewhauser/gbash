package builtins

import (
	"context"
	"errors"
	"io"
	"math"
)

// errFileTooLarge reports a write or resize that would exceed the sandbox
// file-size limit. Its text matches the host EFBIG diagnostic.
var errFileTooLarge = errors.New("File too large")

// fileSizeLimit returns the positive MaxFileBytes bound for inv, or 0 when
// file sizes are unbounded.
func fileSizeLimit(inv *Invocation) int64 {
	if inv == nil {
		return 0
	}
	limit := inv.Limits.MaxFileBytes
	if limit <= 0 || limit == math.MaxInt64 {
		return 0
	}
	return limit
}

// exceedsFileSizeLimit reports whether a file of size bytes would exceed
// inv's MaxFileBytes bound.
func exceedsFileSizeLimit(inv *Invocation, size uint64) bool {
	limit := fileSizeLimit(inv)
	return limit > 0 && size > uint64(limit)
}

// contextReader fails reads once ctx is done so long copy loops stop at the
// request deadline.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// copyBounded copies src to dst, observing ctx between reads and failing
// with errFileTooLarge once more than limit bytes would be written. A
// non-positive limit only applies the context check.
func copyBounded(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	src = contextReader{ctx: ctx, r: src}
	if limit <= 0 {
		return io.Copy(dst, src)
	}
	n, err := io.Copy(dst, io.LimitReader(src, limit))
	if err != nil {
		return n, err
	}
	if n < limit {
		return n, nil
	}
	var probe [1]byte
	m, err := src.Read(probe[:])
	if m > 0 {
		return n, errFileTooLarge
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return n, err
	}
	return n, nil
}
