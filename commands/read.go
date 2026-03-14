package commands

import (
	"context"
	"io"
	"math"
	"strings"

	"github.com/ewhauser/gbash/internal/commandutil"
)

// ReadAll reads reader into memory while honoring ctx cancellation and
// inv.Limits.MaxFileBytes.
//
// Command authors should prefer this over io.ReadAll so large inputs fail
// consistently with built-in commands and produce shell-facing diagnostics.
func ReadAll(ctx context.Context, inv *Invocation, reader io.Reader) ([]byte, error) {
	maxFileBytes := int64(0)
	if inv != nil {
		maxFileBytes = inv.Limits.MaxFileBytes
	}
	return readAllWithLimit(ctx, reader, maxFileBytes)
}

// ReadAllStdin reads inv.Stdin with the same limit and diagnostic behavior as
// [ReadAll]. A nil stdin is treated as an empty reader.
func ReadAllStdin(ctx context.Context, inv *Invocation) ([]byte, error) {
	stdin := io.Reader(strings.NewReader(""))
	if inv != nil && inv.Stdin != nil {
		stdin = inv.Stdin
	}
	return ReadAll(ctx, inv, stdin)
}

func readAllWithLimit(ctx context.Context, reader io.Reader, maxFileBytes int64) ([]byte, error) {
	if reader == nil {
		reader = strings.NewReader("")
	}
	reader = commandutil.ReaderWithContext(ctx, reader)

	if maxFileBytes <= 0 || maxFileBytes == math.MaxInt64 {
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, wrapCommandError(err)
		}
		return data, nil
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxFileBytes+1))
	if err != nil {
		return nil, wrapCommandError(err)
	}
	if int64(len(data)) > maxFileBytes {
		return nil, &ExitError{
			Code: 1,
			Err:  Diagnosticf("input exceeds maximum file size of %d bytes", maxFileBytes),
		}
	}
	return data, nil
}
