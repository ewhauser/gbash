package yq

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	gbruntime "github.com/ewhauser/gbash"
)

// yqSlowLowMemoryScript builds a 300-element array and runs a 27M-iteration
// nested loop over it that never accumulates results.
const yqSlowLowMemoryScript = `echo "[$(seq -s, 1 300)]" > /tmp/n.json` + "\n" +
	`yq -o json '[.[] as $a | .[] as $b | .[] | select(false)]' /tmp/n.json` + "\n"

func runYQWithTimeout(tb testing.TB, rt *gbruntime.Runtime, script string, timeout, grace time.Duration) (*gbruntime.ExecutionResult, time.Duration) {
	tb.Helper()

	type outcome struct {
		result *gbruntime.ExecutionResult
		err    error
	}
	start := time.Now()
	done := make(chan outcome, 1)
	go func() {
		result, err := rt.Run(context.Background(), &gbruntime.ExecutionRequest{Script: script, Timeout: timeout})
		done <- outcome{result: result, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			tb.Fatalf("Run() error = %v", got.err)
		}
		return got.result, time.Since(start)
	case <-time.After(timeout + grace):
		tb.Fatalf("yq did not return within %s of its %s timeout", grace, timeout)
		return nil, 0
	}
}

func TestYQStopsEvaluationOnTimeout(t *testing.T) {
	t.Parallel()

	result, _ := runYQWithTimeout(t, newYQRuntime(t), yqSlowLowMemoryScript, 300*time.Millisecond, 10*time.Second)

	if result.ExitCode != 124 {
		t.Fatalf("ExitCode = %d, want 124; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "execution timed out") {
		t.Fatalf("Stderr = %q, want timeout message", result.Stderr)
	}
}

func TestYQBoundsEvaluationMemory(t *testing.T) {
	t.Parallel()

	for _, expression := range []string{
		// Array doubling: 2^24 elements.
		"[1]" + strings.Repeat(" | . + .", 24),
		// String doubling: 2^30 bytes.
		`"aaaaaaaaaaaaaaaa"` + strings.Repeat(" | . + .", 26),
		// Doubling through in-place assignment.
		`{"a": [1]}` + strings.Repeat(" | .a += .a", 24),
		// Assignment past the end pads the array before any result exists.
		"[1] | .[100000000] = 1",
		"[1] | .[50000000 * 2] = 1",
	} {
		t.Run(expression, func(t *testing.T) {
			t.Parallel()

			result, _ := runYQWithTimeout(t, newYQRuntime(t), "yq -n '"+expression+"'\n", 60*time.Second, 10*time.Second)

			if result.ExitCode != 1 {
				t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "evaluation exceeded") {
				t.Fatalf("Stderr = %q, want evaluation size limit diagnostic", result.Stderr)
			}
		})
	}
}

func TestYQEvaluationInOneRuntimeDoesNotBlockAnother(t *testing.T) {
	t.Parallel()

	slow := make(chan struct{})
	go func() {
		defer close(slow)
		_, _ = newYQRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{Script: yqSlowLowMemoryScript, Timeout: 3 * time.Second})
	}()
	time.Sleep(200 * time.Millisecond)

	result, elapsed := runYQWithTimeout(t, newYQRuntime(t), "yq -n '.a = 1'\n", time.Second, 10*time.Second)
	<-slow

	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "a: 1\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if elapsed > 900*time.Millisecond {
		t.Fatalf("trivial yq took %s while another runtime was evaluating", elapsed)
	}
}

func TestYQLargeLegitimateResultStaysWithinBudget(t *testing.T) {
	t.Parallel()
	session := newYQSession(t)

	result := mustExecSession(t, session, `echo "[$(seq -s, 1 20000)]" > /tmp/n.json`+"\n"+`yq -o json -I 0 '[.[] | {"n": ., "s": "x"}] | length' /tmp/n.json`+"\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "20000\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

// Evaluations no longer serialize on a process-wide lock; run several kinds
// concurrently so the race detector covers yqlib's shared state.
func TestYQConcurrentEvaluationsAcrossRuntimes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		script string
		want   string
	}{
		{script: "printf 'a: 1\\nb: [x, y]\\n' | yq '.b[1]'\n", want: "y\n"},
		{script: "printf '{\"a\":{\"b\":2}}' | yq -p json -o json -I 0 '.a'\n", want: "{\"b\":2}\n"},
		{script: "printf 'a: 1\\n---\\na: 2\\n' | yq ea '[.a] | .[0] + .[1]'\n", want: "3\n"},
		{script: "yq -n '[1,2,3] | map(. * 10) | .[]'\n", want: "10\n20\n30\n"},
	}

	done := make(chan error, 4*len(cases))
	for range 4 {
		for _, tc := range cases {
			go func() {
				result, err := newYQRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{Script: tc.script})
				switch {
				case err != nil:
					done <- err
				case result.ExitCode != 0 || result.Stdout != tc.want:
					done <- fmt.Errorf("%q: exit=%d stdout=%q stderr=%q, want %q", tc.script, result.ExitCode, result.Stdout, result.Stderr, tc.want)
				default:
					done <- nil
				}
			}()
		}
	}
	for range 4 * len(cases) {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestYQAllowsModestArrayIndexAssignment(t *testing.T) {
	t.Parallel()

	result, _ := runYQWithTimeout(t, newYQRuntime(t), "yq -n -o json -I0 '[1] | .[3] = 2'\n", 30*time.Second, 10*time.Second)
	if result.ExitCode != 0 || result.Stdout != "[1,null,null,2]\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}
