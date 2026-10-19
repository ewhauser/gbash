package jq

import (
	"context"
	"strings"
	"testing"

	gbruntime "github.com/ewhauser/gbash"
)

// jqDeepValue builds an array nested far beyond the depth limit without
// recursing in Go, so only the consumers below can overflow the stack.
const jqDeepValue = `reduce range(3000000) as $i (0; [.])`

func TestJQRejectsValuesNestedBeyondDepthLimit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		script string
	}{
		{name: "compact output", script: `jq -c -n '` + jqDeepValue + `'`},
		{name: "pretty output", script: `jq -n '` + jqDeepValue + `'`},
		{name: "tojson", script: `jq -n '` + jqDeepValue + ` | tojson'`},
		{name: "tostring", script: `jq -n '` + jqDeepValue + ` | tostring'`},
		{name: "format", script: `jq -n '` + jqDeepValue + ` | @base64'`},
		{name: "equality", script: `jq -n '` + jqDeepValue + ` as $a | $a == $a'`},
		{name: "ordering", script: `jq -n '` + jqDeepValue + ` as $a | [$a, $a] | sort | length'`},
		{name: "contains", script: `jq -n '` + jqDeepValue + ` as $a | $a | contains($a)'`},
		{name: "flatten", script: `jq -n '` + jqDeepValue + ` | flatten | length'`},
		{name: "object merge", script: `jq -n 'reduce range(3000000) as $i ({}; {a: .}) as $a | $a * $a | length'`},
		{name: "setpath long path", script: `jq -n 'null | setpath([range(3000000) | 0]; 1) | length'`},
		{name: "delpaths deep value", script: `jq -n '` + jqDeepValue + ` | del(.[1]) | length'`},
		{name: "builtin nesting", script: `jq -c -n 'reduce range(3000000) as $i ([0]; to_entries)'`},
		{name: "error message", script: `jq -n '` + jqDeepValue + ` | error'`},
		{name: "pipeline", script: "set -o pipefail\njq -c -n '" + jqDeepValue + "' | cat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := newJQRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
				Script: tc.script + "\necho after=$?\n",
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if !strings.Contains(result.Stdout, "after=") || strings.Contains(result.Stdout, "after=0") {
				t.Fatalf("Stdout = %.200q, want non-zero exit status", result.Stdout)
			}
			if !strings.Contains(result.Stderr, "exceeds depth limit") {
				t.Fatalf("Stderr = %.200q, want depth limit error", result.Stderr)
			}
		})
	}
}

func TestJQAllowsValuesNestedWithinDepthLimit(t *testing.T) {
	t.Parallel()

	result, err := newJQRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script: `jq -c -n 'reduce range(5000) as $i (0; [.]) as $a | [($a | tojson | length), ($a == $a), ($a | flatten), (null | setpath([range(5000) | 0]; 1) | length)]'` + "\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%.200q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "[10001,true,[0],1]\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
