package awk

import (
	"context"
	"strings"
	"testing"
	"time"

	gbruntime "github.com/ewhauser/gbash"
)

func TestAWKStopsInfiniteLoopOnTimeout(t *testing.T) {
	t.Parallel()

	for _, program := range []string{
		`BEGIN { while (1) {} }`,
		`{ while (1) { n++ } }`,
	} {
		t.Run(program, func(t *testing.T) {
			t.Parallel()

			type outcome struct {
				result *gbruntime.ExecutionResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := newAWKRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
					Script:  "echo x | awk '" + program + "'\n",
					Timeout: 200 * time.Millisecond,
				})
				done <- outcome{result: result, err: err}
			}()

			var result *gbruntime.ExecutionResult
			select {
			case got := <-done:
				if got.err != nil {
					t.Fatalf("Run() error = %v", got.err)
				}
				result = got.result
			case <-time.After(10 * time.Second):
				t.Fatalf("awk ignored the 200ms execution timeout")
			}

			if result.ExitCode != 124 {
				t.Fatalf("ExitCode = %d, want 124; stderr=%q", result.ExitCode, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "execution timed out") {
				t.Fatalf("Stderr = %q, want timeout message", result.Stderr)
			}
			if strings.Contains(result.Stderr, "awk:") {
				t.Fatalf("Stderr = %q, want only the runtime timeout message", result.Stderr)
			}
		})
	}
}

func newAWKRuntimeWithMaxDuration(t *testing.T, maxDuration time.Duration) *gbruntime.Runtime {
	t.Helper()

	registry := gbruntime.DefaultRegistry()
	if err := registry.Register(&AWK{maxDuration: maxDuration}); err != nil {
		t.Fatalf("Register(awk) error = %v", err)
	}
	rt, err := gbruntime.New(gbruntime.WithConfig(&gbruntime.Config{Registry: registry}))
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}
	return rt
}

func TestAWKStopsInfiniteLoopWithoutRequestDeadline(t *testing.T) {
	t.Parallel()

	for _, program := range []string{
		`BEGIN { while (1) {} }`,
		`{ while (1) { n++ } }`,
	} {
		t.Run(program, func(t *testing.T) {
			t.Parallel()

			start := time.Now()
			result, err := newAWKRuntimeWithMaxDuration(t, 200*time.Millisecond).Run(context.Background(), &gbruntime.ExecutionRequest{
				Script: "echo x | awk '" + program + "'\necho after=$?\n",
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Fatalf("awk ran for %s past a 200ms limit", elapsed)
			}
			if got, want := result.Stdout, "after=2\n"; got != want {
				t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "time limit") {
				t.Fatalf("Stderr = %q, want time limit error", result.Stderr)
			}
		})
	}
}

func TestAWKTimeBudgetExcludesWaitingOnStdinAndSystem(t *testing.T) {
	t.Parallel()

	result, err := newAWKRuntimeWithMaxDuration(t, 300*time.Millisecond).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script: "{ sleep 0.5; echo x; } | awk '{ system(\"sleep 0.5\"); print }'\necho after=$?\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Stdout, "x\nafter=0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}
