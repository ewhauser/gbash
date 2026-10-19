//go:build cgo && !(darwin && amd64)

package python

import (
	"context"
	"strings"
	"testing"
	"time"

	gbruntime "github.com/ewhauser/gbash"
)

// runPythonLimitScript runs script under a request timeout and fails the test
// instead of hanging when the interpreter ignores the deadline.
func runPythonLimitScript(t *testing.T, script string, timeout time.Duration) *gbruntime.ExecutionResult {
	t.Helper()

	type outcome struct {
		result *gbruntime.ExecutionResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := newPythonRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
			Script:  script,
			Timeout: timeout,
		})
		done <- outcome{result: result, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Run() error = %v", got.err)
		}
		return got.result
	case <-time.After(timeout + 10*time.Second):
		t.Fatalf("python did not stop within %s of the %s request timeout", 10*time.Second, timeout)
		return nil
	}
}

func TestPythonHugeAllocationRaisesMemoryError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		code string
	}{
		{name: "string repeat", code: `x="a"*(10**15)`},
		{name: "huge power", code: `10**(10**12)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runPythonLimitScript(t, "python3 -c '"+tc.code+"'\necho after=$?\n", 30*time.Second)
			if !strings.Contains(result.Stderr, "MemoryError") {
				t.Fatalf("Stderr = %q, want MemoryError", result.Stderr)
			}
			if got, want := result.Stdout, "after=1\n"; got != want {
				t.Fatalf("Stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestPythonDeepRecursionRaisesRecursionError(t *testing.T) {
	t.Parallel()

	result := runPythonLimitScript(t, "python3 -c 'def f(n):\n    return f(n+1)\nf(0)'\necho after=$?\n", 30*time.Second)
	if !strings.Contains(result.Stderr, "RecursionError") {
		t.Fatalf("Stderr = %q, want RecursionError", result.Stderr)
	}
	if got, want := result.Stdout, "after=1\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestPythonHonorsExecutionTimeout(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		script string
	}{
		{name: "busy loop", script: "python3 -c 'while True: pass'\n"},
		{name: "busy loop from stdin", script: "echo 'while True: pass' | python3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start := time.Now()
			result := runPythonLimitScript(t, tc.script, 2*time.Second)
			if elapsed := time.Since(start); elapsed > 6*time.Second {
				t.Fatalf("python ran for %s after a 2s timeout", elapsed)
			}
			if got, want := result.ExitCode, 124; got != want {
				t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "execution timed out") {
				t.Fatalf("Stderr = %q, want execution timeout message", result.Stderr)
			}
		})
	}
}

func TestPythonResourceLimitsWithoutDeadlineUseFallback(t *testing.T) {
	t.Parallel()

	limits, hasDeadline := pythonResourceLimits(context.Background())
	if hasDeadline {
		t.Fatalf("hasDeadline = true, want false")
	}
	if got, want := limits.MaxDuration, pythonDefaultMaxDuration; got != want {
		t.Fatalf("MaxDuration = %s, want %s", got, want)
	}
	if limits.MaxMemory != pythonMaxMemory || limits.MaxRecursionDepth != pythonMaxRecursionDepth {
		t.Fatalf("limits = %+v, want memory and recursion caps", limits)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	limits, hasDeadline = pythonResourceLimits(ctx)
	if !hasDeadline || limits.MaxDuration <= pythonDefaultMaxDuration || limits.MaxDuration > time.Hour {
		t.Fatalf("deadline limits = %+v (hasDeadline=%v), want duration tracking the deadline", limits, hasDeadline)
	}
}

// runPythonCanceled runs script without a request timeout, cancels the
// context after a short delay, and fails instead of hanging when python
// ignores cancellation.
func runPythonCanceled(t *testing.T, run func(context.Context) (*gbruntime.ExecutionResult, error)) (*gbruntime.ExecutionResult, error) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		result *gbruntime.ExecutionResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := run(ctx)
		done <- outcome{result: result, err: err}
	}()

	time.Sleep(500 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case got := <-done:
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("python took %s to stop after cancel", elapsed)
		}
		return got.result, got.err
	case <-time.After(10 * time.Second):
		t.Fatalf("python ignored cancellation for 10s")
		return nil, nil
	}
}

func TestPythonBusyLoopStopsPromptlyOnCancel(t *testing.T) {
	t.Parallel()

	rt := newPythonRuntime(t)
	result, err := runPythonCanceled(t, func(ctx context.Context) (*gbruntime.ExecutionResult, error) {
		return rt.Run(ctx, &gbruntime.ExecutionRequest{Script: "python3 -c 'while True: pass'\n"})
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 130 {
		t.Fatalf("ExitCode = %d, want 130; stderr=%q", result.ExitCode, result.Stderr)
	}
}

func TestPythonREPLBusyLoopStopsPromptlyOnCancel(t *testing.T) {
	t.Parallel()

	session := newPythonSession(t)
	var stdout, stderr strings.Builder
	_, _ = runPythonCanceled(t, func(ctx context.Context) (*gbruntime.ExecutionResult, error) {
		_, err := session.Interact(ctx, &gbruntime.InteractiveRequest{
			Stdin:  strings.NewReader("python3\nwhile True: pass\n\n"),
			Stdout: &stdout,
			Stderr: &stderr,
		})
		return nil, err
	})
}

func TestPythonTTYEnvDoesNotForceREPLForPipedInput(t *testing.T) {
	t.Parallel()

	result, err := newPythonRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script:  "printf 'print(6 * 7)\\n' | TTY=/dev/tty python3\n",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "42\n"; got != want {
		t.Fatalf("Stdout = %q, want %q (no REPL prompts)", got, want)
	}
}
