package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash/policy"
)

// TestCaptureBufferSpill writes more than the limit with a spill dir set and
// verifies the in-memory part is truncated while the overflow lands on disk
// and is exposed via SpillPath.
func TestCaptureBufferSpill(t *testing.T) {
	spillDir := t.TempDir()
	buf := newSpillingCaptureBuffer(16, spillDir)
	payload := strings.Repeat("x", 64)
	if _, err := buf.Write([]byte(payload)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !buf.Truncated() {
		t.Fatal("Truncated() = false, want true")
	}
	if got := buf.String(); got != strings.Repeat("x", 16) {
		t.Fatalf("String() = %q, want 16 x's", got)
	}
	spillPath := buf.SpillPath()
	if spillPath == "" {
		t.Fatal("SpillPath() empty, want spill file")
	}
	if _, err := os.Stat(spillPath); err != nil {
		t.Fatalf("spill file missing: %v", err)
	}
	if err := buf.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	data, err := os.ReadFile(spillPath)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if string(data) != strings.Repeat("x", 48) {
		t.Fatalf("spill file = %d bytes, want 48", len(data))
	}
}

// TestCaptureBufferNoSpillDir keeps the legacy truncate-and-drop behavior when
// no spill directory is configured.
func TestCaptureBufferNoSpillDir(t *testing.T) {
	buf := newCaptureBuffer(8)
	if _, err := buf.Write([]byte("0123456789abcdef")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !buf.Truncated() {
		t.Fatal("Truncated() = false, want true")
	}
	if got := buf.String(); got != "01234567" {
		t.Fatalf("String() = %q, want truncated prefix", got)
	}
	if got := buf.SpillPath(); got != "" {
		t.Fatalf("SpillPath() = %q, want empty without spill dir", got)
	}
}

// TestExecutionTimeoutSetsTimedOut asserts the result distinguishes a timeout
// from a normal exit via the new TimedOut field.
func TestExecutionTimeoutSetsTimedOut(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script:  "while true; do :; done\n",
		Timeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 124 {
		t.Fatalf("ExitCode = %d, want 124", result.ExitCode)
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
}

// TestExecutionCancelNotTimedOut asserts a context cancellation (not a
// timeout) leaves TimedOut false.
func TestExecutionCancelNotTimedOut(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(20*time.Millisecond, cancel)

	result, err := rt.Run(ctx, &ExecutionRequest{
		Script: "while true; do :; done\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 130 {
		t.Fatalf("ExitCode = %d, want 130", result.ExitCode)
	}
	if result.TimedOut {
		t.Fatal("TimedOut = true, want false for cancellation")
	}
}

// TestBigOutputSpillsToDisk runs a script that emits far more than the stdout
// limit with a spill dir, and verifies ExecutionResult exposes the spill path
// with the full output readable from disk.
func TestBigOutputSpillsToDisk(t *testing.T) {
	t.Parallel()
	spillDir := t.TempDir()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxStdoutBytes: 4096})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script:   "i=0; while [ $i -lt 2000 ]; do echo line-$i; i=$((i+1)); done\n",
		SpillDir: spillDir,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.StdoutTruncated {
		t.Fatal("StdoutTruncated = false, want true")
	}
	if result.StdoutSpillPath == "" {
		t.Fatal("StdoutSpillPath empty, want spill file")
	}
	if _, err := os.Stat(result.StdoutSpillPath); err != nil {
		t.Fatalf("spill file missing: %v", err)
	}
	data, err := os.ReadFile(result.StdoutSpillPath)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if !strings.Contains(string(data), "line-1999") {
		t.Fatalf("spill file missing tail line-1999 (len=%d)", len(data))
	}
	if got := result.Stdout; strings.Contains(got, "line-1999") {
		t.Fatal("in-memory Stdout should not contain the tail after truncation")
	}
}

// TestSpillDirPassedThroughNestedExec ensures SpillDir survives the
// commands-layer conversion (nested command execution path).
func TestSpillDirPassedThroughNestedExec(t *testing.T) {
	t.Parallel()
	spillDir := t.TempDir()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxStdoutBytes: 1024})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script:   "echo start; i=0; while [ $i -lt 500 ]; do echo nested-$i; i=$((i+1)); done\n",
		SpillDir: spillDir,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.StdoutSpillPath == "" {
		t.Fatal("StdoutSpillPath empty for nested command execution")
	}
	if filepath.Dir(result.StdoutSpillPath) != spillDir {
		t.Fatalf("spill path %q not under configured dir %q", result.StdoutSpillPath, spillDir)
	}
}
