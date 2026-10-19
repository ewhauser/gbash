package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/policy"
)

// TestCaptureBufferSpill writes more than the limit with a spill dir set and
// verifies the in-memory part is truncated while the overflow lands on disk
// and is exposed via SpillPath.
func TestCaptureBufferSpill(t *testing.T) {
	t.Parallel()
	spillDir := t.TempDir()
	buf := newSpillingCaptureBuffer(16, spillDir, 0)
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
	buf.Close()
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
	t.Parallel()
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

// TestCaptureBufferSpillCreateFailureFallsBack asserts an unusable spill dir
// degrades to truncation instead of failing the write.
func TestCaptureBufferSpillCreateFailureFallsBack(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	buf := newSpillingCaptureBuffer(8, missing, 0)
	for range 3 {
		n, err := buf.Write([]byte("0123456789abcdef"))
		if err != nil {
			t.Fatalf("Write() error = %v, want nil", err)
		}
		if n != 16 {
			t.Fatalf("Write() n = %d, want 16", n)
		}
	}
	if got := buf.String(); got != "01234567" {
		t.Fatalf("String() = %q, want truncated prefix", got)
	}
	if got := buf.SpillPath(); got != "" {
		t.Fatalf("SpillPath() = %q, want empty after spill failure", got)
	}
}

// TestCaptureBufferSpillCap asserts the spill file stops growing at spillMax.
func TestCaptureBufferSpillCap(t *testing.T) {
	t.Parallel()
	spillDir := t.TempDir()
	buf := newSpillingCaptureBuffer(4, spillDir, 10)
	for range 5 {
		if _, err := buf.Write([]byte("abcdefgh")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	buf.Close()
	data, err := os.ReadFile(buf.SpillPath())
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if got, want := string(data), "efghabcdef"; got != want {
		t.Fatalf("spill file = %q, want %q", got, want)
	}
	if !buf.SpillTruncated() {
		t.Fatal("SpillTruncated() = false, want true after hitting spillMax")
	}
}

// TestCaptureBufferSpillExactlyAtCapNotTruncated asserts overflow that fits
// the cap exactly is not reported as truncated, while one more byte is.
func TestCaptureBufferSpillExactlyAtCapNotTruncated(t *testing.T) {
	t.Parallel()
	buf := newSpillingCaptureBuffer(4, t.TempDir(), 10)
	if _, err := buf.Write([]byte("abcd0123456789")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if buf.SpillTruncated() {
		t.Fatal("SpillTruncated() = true, want false when overflow fits spillMax")
	}
	if _, err := buf.Write([]byte("x")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	buf.Close()
	if !buf.SpillTruncated() {
		t.Fatal("SpillTruncated() = false, want true once overflow exceeds spillMax")
	}
}

// TestExecutionReportsSpillTruncation asserts results flag a spill file that
// was cut short at MaxSpillBytes, per stream.
func TestExecutionReportsSpillTruncation(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxStdoutBytes: 4, MaxStderrBytes: 4})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script:        "printf '%040d' 0\nprintf 'abcdefgh' >&2\n",
		SpillDir:      t.TempDir(),
		MaxSpillBytes: 10,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.StdoutSpillPath == "" || !result.StdoutSpillTruncated {
		t.Fatalf("stdout spill = (%q, truncated=%t), want truncated spill file", result.StdoutSpillPath, result.StdoutSpillTruncated)
	}
	data, err := os.ReadFile(result.StdoutSpillPath)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if len(data) != 10 {
		t.Fatalf("stdout spill file = %d bytes, want 10", len(data))
	}
	if result.StderrSpillPath == "" || result.StderrSpillTruncated {
		t.Fatalf("stderr spill = (%q, truncated=%t), want complete spill file", result.StderrSpillPath, result.StderrSpillTruncated)
	}
}

// TestCaptureBufferNoSpillAfterClose asserts late writes cannot create a new,
// unreported spill file once the buffer is closed.
func TestCaptureBufferNoSpillAfterClose(t *testing.T) {
	t.Parallel()
	spillDir := t.TempDir()
	buf := newSpillingCaptureBuffer(4, spillDir, 0)
	buf.Close()
	if _, err := buf.Write([]byte("0123456789")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := buf.SpillPath(); got != "" {
		t.Fatalf("SpillPath() = %q, want empty after Close", got)
	}
	entries, err := os.ReadDir(spillDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("spill dir has %d entries, want 0", len(entries))
	}
}

// TestExecutionTimeoutSetsTimedOut asserts the result distinguishes a timeout
// from a normal exit via the new TimedOut field.
func TestExecutionTimeoutSetsTimedOut(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, unboundedLoopLimits)

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
	rt := newRuntimeWithLimits(t, unboundedLoopLimits)

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

// TestExecutionTimeoutMessageSpilled asserts the timeout message is reported
// via StderrSpillPath when stderr was already full when the deadline hit.
func TestExecutionTimeoutMessageSpilled(t *testing.T) {
	t.Parallel()
	spillDir := t.TempDir()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxStderrBytes: 8})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script:   "printf 'xxxxxxxx' >&2\nwhile true; do :; done\n",
		Timeout:  20 * time.Millisecond,
		SpillDir: spillDir,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
	if result.StderrSpillPath == "" {
		t.Fatal("StderrSpillPath empty, want spilled timeout message")
	}
	data, err := os.ReadFile(result.StderrSpillPath)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if !strings.Contains(string(data), "execution timed out") {
		t.Fatalf("spill file = %q, want timeout message", data)
	}
}

// TestBigOutputSpillsToDisk runs a script that emits far more than the stdout
// limit with a spill dir, and verifies Stdout plus the spill file reproduce
// the full output.
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
	data, err := os.ReadFile(result.StdoutSpillPath)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	var want strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&want, "line-%d\n", i)
	}
	if got := result.Stdout + string(data); got != want.String() {
		t.Fatalf("Stdout+spill = %d bytes, want %d", len(got), want.Len())
	}
}

// TestSpillDirPassedThroughNestedExec ensures SpillDir survives the
// commands-layer conversion for a nested Invocation.Exec.
func TestSpillDirPassedThroughNestedExec(t *testing.T) {
	t.Parallel()
	spillDir := t.TempDir()
	probe := commands.DefineCommand("spillprobe", func(ctx context.Context, inv *commands.Invocation) error {
		result, err := inv.Exec(ctx, &commands.ExecutionRequest{
			Script:   "i=0; while [ $i -lt 500 ]; do echo nested-$i; i=$((i+1)); done\n",
			SpillDir: spillDir,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(inv.Stdout, result.StdoutSpillPath)
		return err
	})
	rt := newRuntime(t, &Config{
		Registry: registryWithCommands(t, probe),
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits:     policy.Limits{MaxStdoutBytes: 1024},
		}),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{Script: "spillprobe\n"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	spillPath := strings.TrimSpace(result.Stdout)
	if spillPath == "" {
		t.Fatal("nested StdoutSpillPath empty, want spill file")
	}
	if filepath.Dir(spillPath) != spillDir {
		t.Fatalf("spill path %q not under configured dir %q", spillPath, spillDir)
	}
}
