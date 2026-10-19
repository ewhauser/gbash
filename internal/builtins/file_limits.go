package builtins

import (
	"context"
	"errors"
	"io"
	"math"
)

// errFileTooLarge reports a write or resize that would exceed the sandbox
// file-size limit. Its text matches the GNU EFBIG diagnostic.
var errFileTooLarge error = fileTooLargeError{}

type fileTooLargeError struct{}

func (fileTooLargeError) Error() string { return "File too large" }

// maxOutputNameWidth bounds user-requested file-name suffix widths (split -a,
// csplit -n and --suffix-format widths) so a huge width cannot allocate
// gigabytes for one name. It matches the usual PATH_MAX.
const maxOutputNameWidth = 4096

// unboundedAllocationCap bounds buffers sized directly by user input when
// MaxFileBytes is unlimited, so a request cannot trigger a fatal Go
// out-of-memory that kills the embedding process.
const unboundedAllocationCap = 1 << 30

// allocationLimit returns the largest buffer a builtin may allocate from a
// user-supplied size: MaxFileBytes when bounded, else unboundedAllocationCap.
func allocationLimit(inv *Invocation) int64 {
	if limit := fileSizeLimit(inv); limit > 0 {
		return limit
	}
	return unboundedAllocationCap
}

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

// copyBounded copies src to dst, observing ctx between reads and failing
// with errFileTooLarge once more than limit bytes would be written. A
// non-positive limit only applies the context check.
func copyBounded(ctx context.Context, dst io.Writer, src io.Reader, limit int64) error {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if limit > 0 && written+int64(n) > limit {
				return errFileTooLarge
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
