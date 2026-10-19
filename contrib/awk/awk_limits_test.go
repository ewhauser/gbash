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
