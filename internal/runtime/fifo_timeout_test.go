package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash/policy"
)

func TestExecutionTimeoutInterruptsFIFOWriteWithoutReader(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{})

	done := make(chan struct{})
	var (
		result *ExecutionResult
		err    error
	)
	go func() {
		defer close(done)
		result, err = rt.Run(context.Background(), &ExecutionRequest{
			Script:  "mkfifo /tmp/p\necho hi > /tmp/p\necho after\n",
			Timeout: 50 * time.Millisecond,
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after the execution timeout")
	}
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 124 {
		t.Fatalf("ExitCode = %d, want 124; stderr=%q", result.ExitCode, result.Stderr)
	}
	if strings.Contains(result.Stdout, "after") {
		t.Fatalf("Stdout = %q, want no output after the timed-out write", result.Stdout)
	}
}

func TestExecutionCancelInterruptsFIFOReadWithoutWriter(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)

	done := make(chan struct{})
	var (
		result *ExecutionResult
		err    error
	)
	go func() {
		defer close(done)
		result, err = rt.Run(ctx, &ExecutionRequest{
			Script: "mkfifo /tmp/p\ncat /tmp/p\necho after\n",
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 130 {
		t.Fatalf("ExitCode = %d, want 130; stderr=%q", result.ExitCode, result.Stderr)
	}
}
