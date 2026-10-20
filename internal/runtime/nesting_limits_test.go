package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash/policy"
)

const (
	subshellDepthMessage = "subshell nesting limit exceeded ("
	functionDepthMessage = "maximum function nesting level exceeded ("
)

func runNestingScript(t *testing.T, rt *Runtime, script string) *ExecutionResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := rt.Run(ctx, &ExecutionRequest{Script: script})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result
}

func TestSubshellDepthDefaultStopsRecursiveSubshells(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	for _, script := range []string{
		// Each of these used to grow memory without bound (20 GB and more).
		"f(){ (f); }; f; echo \"after $?\"\n",
		"f(){ echo $(f); }; f >/dev/null; echo \"after $?\"\n",
		"f(){ echo `f`; }; f >/dev/null; echo \"after $?\"\n",
		"f(){ cat <(f); }; f >/dev/null; echo \"after $?\"\n",
		strings.Repeat("( ", 10000) + "true" + strings.Repeat(" )", 10000) + "\necho \"after $?\"\n",
	} {
		result := runNestingScript(t, rt, script)
		if !strings.Contains(result.Stdout, "after ") {
			t.Fatalf("script %.40q: Stdout = %q, want the outer shell to continue", script, result.Stdout)
		}
		if want := subshellDepthMessage + "50), increase policy.Limits.MaxSubstitutionDepth"; !strings.Contains(result.Stderr, want) {
			t.Fatalf("script %.40q: Stderr = %q, want %q", script, result.Stderr, want)
		}
	}
}

func TestSubshellDepthReportsStatus126(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxSubstitutionDepth: 3})

	result := runNestingScript(t, rt, "f(){ echo $1; (f $(( $1 + 1 ))); }; f 0; echo \"after $?\"\n")
	if got, want := result.Stdout, "0\n1\n2\n3\nafter 126\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if want := subshellDepthMessage + "3)"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestSubshellDepthIgnoresPipelineStages(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// Pipeline stages run side by side, so a long pipeline is not nesting.
	result := runNestingScript(t, rt, "echo hi"+strings.Repeat(" | cat", 200)+"\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "hi\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestFunctionDepthDefaultStopsRecursion(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result := runNestingScript(t, rt, "f(){ f; }; f\necho unreachable\n")
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "" {
		t.Fatalf("Stdout = %q, want empty", result.Stdout)
	}
	if want := "f: " + functionDepthMessage + "1000), increase policy.Limits.MaxFunctionDepth"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestFunctionDepthDefaultAllowsDeepRecursion(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	start := time.Now()
	result := runNestingScript(t, rt, "n=0; f(){ n=$((n+1)); [ $n -lt 1000 ] && f; }; f; echo $n\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "1000\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	// Variable lookups and writes from deep function scopes used to walk
	// every scope, which took over ten seconds here without the race detector
	// and about ten seconds with it in CI. It now takes well under a second.
	// TestFunctionRecursionCostIsLinearInDepth checks the growth directly.
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("recursion to depth 1000 took %v, want well under 10s", elapsed)
	}
}

func TestFunctionRecursionCostIsLinearInDepth(t *testing.T) { //nolint:paralleltest // timing ratios need the package to themselves
	rt := newRuntimeWithLimits(t, policy.Limits{MaxFunctionDepth: 2000})

	const shallow, deep = 500, 2000
	for _, tc := range []struct { //nolint:paralleltest // see above
		name   string
		script string
	}{
		{"globals", "n=0; f(){ n=$((n+1)); [ $n -lt %d ] && f; }; f; echo $n\n"},
		{"temp bindings", "n=0; f(){ n=$((n+1)); [ $n -lt %d ] && y=$n f; }; f; echo $n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(depth int) time.Duration {
				start := time.Now()
				result := runNestingScript(t, rt, fmt.Sprintf(tc.script, depth))
				elapsed := time.Since(start)
				if got, want := result.Stdout, fmt.Sprintf("%d\n", depth); got != want {
					t.Fatalf("depth %d: Stdout = %q, want %q; stderr=%q", depth, got, want, result.Stderr)
				}
				return elapsed
			}
			// Take the fastest of a few interleaved runs to shed scheduling
			// and GC noise.
			fastest := [2]time.Duration{time.Hour, time.Hour}
			for range 3 {
				for i, depth := range []int{shallow, deep} {
					fastest[i] = min(fastest[i], run(depth))
				}
			}
			// Four times the depth should cost about four times as much.
			// Scope walks made it sixteen; allow some slack for the Go
			// runtime scanning the deeper goroutine stack.
			ratio := float64(fastest[1]) / float64(fastest[0])
			if ratio > 8 {
				t.Fatalf("depth %d took %v but depth %d took %v (%.1fx), want about 4x", deep, fastest[1], shallow, fastest[0], ratio)
			}
			t.Logf("depth %d: %v, depth %d: %v (%.1fx)", shallow, fastest[0], deep, fastest[1], ratio)
		})
	}
}

func TestFunctionDepthCountsAcrossSubshells(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxFunctionDepth: 5})

	result := runNestingScript(t, rt, "f(){ echo $1; g $(( $1 + 1 )); }; g(){ ( f $1 ); }; f 0; echo \"after $?\"\n")
	if got, want := result.Stdout, "0\n1\n2\nafter 126\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if want := functionDepthMessage + "5)"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestNestedShellsShareCommandBudget(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// Each bash -c used to start with a fresh budget, so six of them ran
	// 54,000 commands under the default limit of 10,000.
	result := runNestingScript(t, rt, `for j in 1 2 3 4 5 6; do bash -c "for i in {1..9000}; do :; done"; echo "j=$j"; done`+"\n")
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "j=1\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if want := "too many commands executed (>10000)"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestNestedShellsCountTowardSubshellDepth(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// This used to recurse for minutes while memory grew past a gigabyte.
	start := time.Now()
	result := runNestingScript(t, rt, `export x='bash -c "$x"'; bash -c "$x"; echo "after $?"`+"\n")
	if got, want := result.Stdout, "after 126\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if want := subshellDepthMessage + "50)"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("recursive bash -c took %v, want well under 10s", elapsed)
	}
}

func TestNestedShellDepthIncludesCallerSubshells(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxSubstitutionDepth: 3})

	result := runNestingScript(t, rt, `echo $(echo $(bash -c 'echo ok')); echo $( ( ( bash -c 'echo deep' ) ) ); echo "after $?"`+"\n")
	if got, want := result.Stdout, "ok\n\nafter 0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if want := subshellDepthMessage + "3)"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}
