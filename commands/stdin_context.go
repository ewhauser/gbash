package commands

import (
	"context"
	"io"
)

// CloseReaderOnContext arranges for cancellation to close a command stdin
// reader when that reader supports io.Closer. This lets blocked in-process
// commands observing pipeline input unblock promptly when a timeout expires.
func CloseReaderOnContext(ctx context.Context, reader io.Reader) {
	if ctx == nil || reader == nil {
		return
	}
	closer, ok := reader.(io.Closer)
	if !ok {
		return
	}
	context.AfterFunc(ctx, func() {
		_ = closer.Close()
	})
}
