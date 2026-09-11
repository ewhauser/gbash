package runtime

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type captureBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int64
	truncated bool

	// Optional spill: when non-empty, output beyond limit is written to a
	// temp file instead of being dropped, and the result exposes the path so
	// callers can read the full output. Empty disables spilling (legacy
	// behavior: overflow is truncated and discarded).
	spillDir  string
	spillPath string
	spillFile *os.File
}

func newCaptureBuffer(limit int64) *captureBuffer {
	return &captureBuffer{limit: limit}
}

func newSpillingCaptureBuffer(limit int64, spillDir string) *captureBuffer {
	return &captureBuffer{limit: limit, spillDir: spillDir}
}

func (b *captureBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Unbounded: keep everything in memory (no truncation, no spill).
	if b.limit <= 0 {
		return b.buf.Write(p)
	}

	remaining := int(b.limit) - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		if err := b.spillLocked(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}

	if len(p) <= remaining {
		return b.buf.Write(p)
	}

	b.truncated = true
	if _, err := b.buf.Write(p[:remaining]); err != nil {
		return 0, err
	}
	if err := b.spillLocked(p[remaining:]); err != nil {
		return 0, err
	}
	return len(p), nil
}

// spillLocked writes overflow to the spill temp file, creating it on first
// use. No-op when spilling is disabled. Callers must hold b.mu.
func (b *captureBuffer) spillLocked(p []byte) error {
	if b.spillDir == "" {
		return nil
	}
	if b.spillFile == nil {
		f, err := os.CreateTemp(b.spillDir, "gbash-output-*")
		if err != nil {
			return fmt.Errorf("gbash: create spill file: %w", err)
		}
		b.spillFile = f
		b.spillPath = f.Name()
	}
	if _, err := b.spillFile.Write(p); err != nil {
		return fmt.Errorf("gbash: write spill file: %w", err)
	}
	return nil
}

func (b *captureBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *captureBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// SpillPath returns the temp file holding the overflow, or "" when nothing
// was spilled (output fit within the limit, or spilling is disabled).
func (b *captureBuffer) SpillPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spillPath
}

// Close flushes and closes the spill file if one was created. Safe to call
// multiple times; no-op when no spill file exists.
func (b *captureBuffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.spillFile == nil {
		return nil
	}
	err := b.spillFile.Close()
	b.spillFile = nil
	if err != nil {
		return fmt.Errorf("gbash: close spill file: %w", err)
	}
	return nil
}

// spillDirFor returns the spill directory from the config, or "" when the
// config disables spilling.
func spillDirFor(dir string) string {
	return filepath.Clean(dir)
}
