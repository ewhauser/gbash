package runtime

import (
	"bytes"
	"os"
	"sync"
)

// defaultMaxSpillBytes caps each spill file when the request does not set
// MaxSpillBytes, so spilling never turns into unbounded host disk usage.
const defaultMaxSpillBytes int64 = 64 << 20

type captureBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int64
	truncated bool

	// Optional spill: when spillDir is non-empty, output beyond limit is
	// written to a host temp file (up to spillMax bytes) instead of being
	// dropped. The file holds only the overflow, so the in-memory buffer
	// followed by the file reproduces the full stream. Spill failures never
	// surface as write errors: the partial file is removed and the buffer falls
	// back to truncate-and-drop.
	spillDir   string
	spillMax   int64
	spillPath  string
	spillFile  *os.File
	spillBytes int64
	spillDone  bool
	// spillCut records that overflow was dropped because the spill file
	// reached spillMax.
	spillCut bool
}

func newCaptureBuffer(limit int64) *captureBuffer {
	return &captureBuffer{limit: limit}
}

func newSpillingCaptureBuffer(limit int64, spillDir string, spillMax int64) *captureBuffer {
	if spillMax <= 0 {
		spillMax = defaultMaxSpillBytes
	}
	return &captureBuffer{limit: limit, spillDir: spillDir, spillMax: spillMax}
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
		b.spillLocked(p)
		return len(p), nil
	}

	if len(p) <= remaining {
		return b.buf.Write(p)
	}

	b.truncated = true
	_, _ = b.buf.Write(p[:remaining])
	b.spillLocked(p[remaining:])
	return len(p), nil
}

// spillLocked writes overflow to the spill temp file, creating it on first
// use. No-op when spilling is disabled, has failed, has hit spillMax, or the
// buffer was closed. Callers must hold b.mu.
func (b *captureBuffer) spillLocked(p []byte) {
	if b.spillDir == "" || b.spillDone {
		return
	}
	if b.spillFile == nil {
		f, err := os.CreateTemp(b.spillDir, "gbash-output-*") //nolint:forbidigo // spill files live on the host in the caller-provided SpillDir, outside the sandbox FS.
		if err != nil {
			b.spillDone = true
			return
		}
		b.spillFile = f
		b.spillPath = f.Name()
	}
	if room := b.spillMax - b.spillBytes; int64(len(p)) > room {
		p = p[:room]
		b.spillDone = true
		b.spillCut = true
	}
	n, err := b.spillFile.Write(p)
	b.spillBytes += int64(n)
	if err != nil {
		b.abandonSpillLocked()
	}
}

// abandonSpillLocked discards a spill file that could not be written in full
// so callers never read a silently incomplete overflow. Callers must hold b.mu.
func (b *captureBuffer) abandonSpillLocked() {
	b.spillDone = true
	if b.spillFile != nil {
		_ = b.spillFile.Close()
		b.spillFile = nil
	}
	if b.spillPath != "" {
		_ = os.Remove(b.spillPath) //nolint:forbidigo // removes the host-side spill file this buffer created.
		b.spillPath = ""
	}
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
// was spilled (output fit within the limit, spilling is disabled, or the spill
// failed).
func (b *captureBuffer) SpillPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spillPath
}

// SpillTruncated reports that the spill file exists but stops short of the
// full stream because the overflow exceeded spillMax.
func (b *captureBuffer) SpillTruncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spillCut && b.spillPath != ""
}

// Close closes the spill file if one was created and stops further spilling,
// so late writes cannot create an unreported file. Safe to call multiple
// times. A failed close removes the spill file, as with a failed write.
func (b *captureBuffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.spillFile == nil {
		b.spillDone = true
		return
	}
	err := b.spillFile.Close()
	b.spillFile = nil
	b.spillDone = true
	if err != nil {
		b.abandonSpillLocked()
	}
}
