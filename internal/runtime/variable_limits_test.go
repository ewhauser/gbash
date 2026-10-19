package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

const variableLimitHint = "increase policy.Limits.MaxVariableBytes"

// variableLimitPrelude defines a 40-byte value, so doubling it exceeds the
// 64-byte limit the path tests run under.
const variableLimitPrelude = "y=0123456789012345678901234567890123456789\n"

func TestMaxVariableBytesDefaultStopsStringDoubling(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// Without a cap this reaches a 128 MB value in 27 iterations and keeps
	// doubling from there.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "x=a; for i in $(seq 40); do x=$x$x; done; echo ${#x}\necho unreachable\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "" {
		t.Fatalf("Stdout = %q, want empty", result.Stdout)
	}
	if want := "x: value too large (limit 33554432 bytes), " + variableLimitHint; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestMaxVariableBytesDefaultAllowsLargeValues(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// 8 MiB matches the default MaxFileBytes, so a whole file still fits.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "x=a; for i in $(seq 23); do x=$x$x; done; y=$(printf '%s' \"$x\"); a=(\"$x\" \"$x\"); echo ${#x} ${#y} ${#a[1]}\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "8388608 8388608 8388608\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestMaxVariableBytesOverride(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		LimitOverrides: policy.Limits{MaxVariableBytes: 64},
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: variableLimitPrelude + "x=$y; echo ${#x}\nx=$y$y\necho unreachable\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "40\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if want := "x: value too large (limit 64 bytes), " + variableLimitHint; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestMaxVariableBytesCoversAssignmentPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		what   string
	}{
		{name: "plain", script: "x=$y$y", what: "x"},
		{name: "append", script: "x=$y; x+=$y", what: "x"},
		{name: "array element", script: "a[3]=$y$y", what: "a"},
		{name: "array element append", script: "a=($y); a[0]+=$y", what: "a"},
		{name: "array literal", script: "a=($y $y)", what: "a"},
		{name: "array append", script: "a=($y); a+=($y)", what: "a"},
		{name: "many small elements", script: "a=(1 2 3 4 5)", what: "a"},
		{name: "associative element", script: "declare -A m; m[k]=$y$y", what: "m"},
		{name: "read", script: "read -r x <<< \"$y$y\"", what: "read"},
		{name: "read array", script: "read -ra x <<< \"$y $y\"", what: "read"},
		{name: "printf -v", script: "printf -v x '%s%s' \"$y\" \"$y\"", what: "x"},
		{name: "declare", script: "declare x=$y$y", what: "x"},
		{name: "declare array", script: "declare -a a=($y $y)", what: "a"},
		{name: "local", script: "f() { local x=$y$y; }; f", what: "x"},
		{name: "export", script: "export x=$y$y", what: "x"},
		{name: "readonly", script: "readonly x=$y$y", what: "x"},
		{name: "mapfile", script: "mapfile -t a < <(echo $y; echo $y)", what: "a"},
		{name: "mapfile long record", script: "mapfile a <<< \"$y$y\"", what: "mapfile"},
		{name: "readarray", script: "readarray -t a < <(echo $y; echo $y)", what: "a"},
		{name: "default expansion", script: ": ${x:=$y$y}", what: "x"},
		{name: "for variable", script: "for x in $y$y; do :; done", what: "x"},
		{name: "eval", script: "eval 'x=$y$y'", what: "x"},
		{name: "function body", script: "f() { x=$y$y; }; f", what: "x"},
		{name: "command substitution", script: "x=$(echo $y$y)", what: "command substitution"},
		{name: "unassigned command substitution", script: "echo \"$(echo $y$y)\"", what: "command substitution"},
		{name: "pattern substitution", script: "echo ${y//?/$y}", what: "pattern substitution"},
		{name: "array pattern substitution", script: "a=($y); echo ${a[@]//?/$y}", what: "pattern substitution"},
		{name: "or list", script: "x=$y$y || echo handled", what: "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 64})
			result, err := rt.Run(context.Background(), &ExecutionRequest{
				Script: variableLimitPrelude + tc.script + "\necho unreachable\n",
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.ExitCode != 126 {
				t.Fatalf("ExitCode = %d, want 126; stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
			}
			if result.Stdout != "" {
				t.Fatalf("Stdout = %q, want empty", result.Stdout)
			}
			want := tc.what + ": value too large (limit 64 bytes), " + variableLimitHint
			if got := strings.Count(result.Stderr, want); got != 1 {
				t.Fatalf("Stderr = %q, want %q once", result.Stderr, want)
			}
		})
	}
}

func TestMaxVariableBytesLeavesValueUnchanged(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 64})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: variableLimitPrelude + "x=keep; trap 'echo trap x=$x' EXIT\nx=$y$y\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "trap x=keep\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestMaxVariableBytesInSubshellAndCommandSubstitution(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 64})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: variableLimitPrelude +
			"( x=$y$y; echo no ); echo sub=$?\n" +
			"z=$(x=$y$y; echo no); echo cmd=$? z=$z\n" +
			"echo $y$y | { read -r x; echo no; }; echo pipe=$?\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "sub=126\ncmd=126 z=\npipe=126\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := strings.Count(result.Stderr, variableLimitHint); got != 3 {
		t.Fatalf("Stderr = %q, want three limit messages", result.Stderr)
	}
}

func TestMaxVariableBytesStopsUnboundedCommandSubstitution(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 1 << 10, MaxLoopIterations: 1 << 20})

	// The producer never stops on its own; the capped capture must end it.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "x=$(while :; do echo yyyyyyyy || break; done)\necho unreachable\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if want := "command substitution: value too large (limit 1024 bytes), " + variableLimitHint; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestMaxVariableBytesAllowsValuesAtLimit(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 80})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: variableLimitPrelude +
			"x=$y$y; z=$(printf %s $y$y); read -r r <<< \"$x\"; printf -v p %s \"$x\"; w=${y//0/00}\n" +
			"a=(1 2 3 4); mapfile -t m < <(echo 1; echo 2; echo 3; echo 4)\n" +
			"echo ${#x} ${#z} ${#r} ${#p} ${#w} ${#a[@]} ${#m[@]}\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "80 80 80 80 44 4 4\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestMaxVariableBytesSequentialArrayAppendStaysLinear(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// Copying the array once per element made this quadratic, so the cap was
	// never reached in reasonable time.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "a=(x); for i in $(seq 40); do a+=(\"${a[@]}\"); done\necho unreachable\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if want := "a: value too large (limit 33554432 bytes), " + variableLimitHint; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}
