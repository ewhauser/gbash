package jq

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gbruntime "github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/contrib/jq/internal/gojq"
)

func runJQLimitScript(t *testing.T, script string) *gbruntime.ExecutionResult {
	t.Helper()

	result, err := newJQRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script: script + "\necho after=$?\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result
}

func TestJQRejectsOversizedValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		filter string
	}{
		{name: "string repeat", filter: `"x" * 400000000 | length`},
		{name: "string concat", filter: `("x" * 40000000) as $s | $s + $s | length`},
		{name: "array concat", filter: `[range(3000000)] as $a | $a + $a | length`},
		{name: "collect range", filter: `[range(100000000)] | length`},
		{name: "flatten", filter: `[range(3000000)] as $a | [$a, $a] | flatten | length`},
		{name: "join", filter: `("x" * 40000000) as $s | [$s, $s] | join("") | length`},
		{name: "tojson", filter: `("x" * 40000000) as $s | [$s, $s] | tojson | length`},
		{name: "format", filter: `("x" * 40000000) as $s | [$s, $s] | @csv | length`},
		{name: "split", filter: `"x" * 60000000 | split("") | length`},
		{name: "explode", filter: `"x" * 60000000 | explode | length`},
		{name: "not catchable", filter: `try ("x" * 400000000) catch "caught"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runJQLimitScript(t, "jq -n '"+tc.filter+"'")
			if got, want := result.Stdout, "after=5\n"; got != want {
				t.Fatalf("Stdout = %.200q, want %q; stderr=%.200q", got, want, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "exceeds size limit") {
				t.Fatalf("Stderr = %.200q, want size limit error", result.Stderr)
			}
		})
	}
}

func TestJQAllowsValuesWithinSizeLimits(t *testing.T) {
	t.Parallel()

	result := runJQLimitScript(t, `jq -n '("x" * 1000000 | length), ([range(1000000)] | length), ([range(100000)] | map(tostring) | join(",") | length)'`)
	if got, want := result.Stdout, "1000000\n1000000\n588889\nafter=0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func TestJQEvalContextStopsRunawayFiltersWithoutDeadline(t *testing.T) {
	t.Parallel()

	for _, filter := range []string{`repeat(.)`, `def f: f; f`, `last(range(infinite))`} {
		t.Run(filter, func(t *testing.T) {
			t.Parallel()

			query, err := gojq.Parse(filter)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			code, err := gojq.Compile(query)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			evalCtx, cancel := jqEvalContextWithLimit(context.Background(), 200*time.Millisecond)
			defer cancel()

			start := time.Now()
			iter := code.RunWithContext(evalCtx, 1)
			for {
				value, ok := iter.Next()
				if !ok {
					t.Fatalf("iterator ended without a time limit error")
				}
				if _, isErr := value.(error); isErr {
					break
				}
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("filter ran for %s past a 200ms limit", elapsed)
			}
			if err := jqEvalLimitError(evalCtx); !errors.Is(err, errJQTimeLimit) {
				t.Fatalf("jqEvalLimitError() = %v, want %v", err, errJQTimeLimit)
			}
		})
	}
}

func TestJQHonorsRequestTimeout(t *testing.T) {
	t.Parallel()

	start := time.Now()
	result, err := newJQRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script:  "jq -n 'def f: f; f'\n",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("jq ran for %s after a 1s timeout", elapsed)
	}
	if got, want := result.ExitCode, 124; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}
}

func TestJQSetpathRespectsArrayLimit(t *testing.T) {
	t.Parallel()

	for _, filter := range []string{`[1] | .[100000000] = 1`, `setpath([100000000]; 1) | length`} {
		result := runJQLimitScript(t, "jq -n '"+filter+"'")
		if got, want := result.Stdout, "after=5\n"; got != want {
			t.Fatalf("%s: Stdout = %.200q, want %q; stderr=%.200q", filter, got, want, result.Stderr)
		}
		if !strings.Contains(result.Stderr, "exceeds size limit") {
			t.Fatalf("%s: Stderr = %.200q, want size limit error", filter, result.Stderr)
		}
	}
}

func TestJQRejectsDeeplyNestedFilter(t *testing.T) {
	t.Parallel()

	filter := strings.Repeat("(", 1_000_000) + "1" + strings.Repeat(")", 1_000_000)
	result := runJQLimitScript(t, "jq -n '"+filter+"'")
	if !strings.Contains(result.Stdout, "after=") || strings.Contains(result.Stdout, "after=0") {
		t.Fatalf("Stdout = %.200q, want failure; stderr=%.200q", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "depth limit") {
		t.Fatalf("Stderr = %.200q, want depth limit error", result.Stderr)
	}
}

func TestJQStreamBoundsPathCopies(t *testing.T) {
	t.Parallel()

	result := runJQLimitScript(t, "jq -nc 'reduce range(3000) as $i ([range(3000)]; [.])' | jq --stream -c . >/dev/null")
	if got, want := result.Stdout, "after=5\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%.200q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "path elements") {
		t.Fatalf("Stderr = %.200q, want stream budget error", result.Stderr)
	}
}
