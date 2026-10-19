package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

const variableLimitHint = "increase policy.Limits.MaxVariableBytes"

// variableLimitPrelude defines a 40-byte y and an 80-byte z. Under the
// 100-byte limit the path tests use, $z$y overflows while a command with
// one y argument still fits the argument-list cap.
const variableLimitPrelude = fortyBytePrelude + "z=$y$y\n"

// fortyBytePrelude defines only the 40-byte y, for tests under smaller limits.
const fortyBytePrelude = "y=0123456789012345678901234567890123456789\n"

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
		Script: fortyBytePrelude + "x=$y; echo ${#x}\nx=$y$y\necho unreachable\n",
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
		{name: "plain", script: "x=$z$y", what: "x"},
		{name: "append", script: "x=$z; x+=$y", what: "x"},
		{name: "array element", script: "a[3]=$z$y", what: "a"},
		{name: "array element append", script: "a=($z); a[0]+=$y", what: "a"},
		{name: "array literal", script: "a=($z $y)", what: "a"},
		{name: "array append", script: "a=($z); a+=($y)", what: "a"},
		{name: "many small elements", script: "a=(1 2 3 4 5 6)", what: "a"},
		{name: "associative element", script: "declare -A m; m[k]=$z$y", what: "m"},
		{name: "read", script: "read -r x <<< \"$z$y\"", what: "read"},
		{name: "read array", script: "read -ra x <<< \"$z $y\"", what: "read"},
		{name: "declare", script: "declare x=$z$y", what: "x"},
		{name: "declare array", script: "declare -a a=($z $y)", what: "a"},
		{name: "local", script: "f() { local x=$z$y; }; f", what: "x"},
		{name: "export", script: "export x=$z$y", what: "x"},
		{name: "readonly", script: "readonly x=$z$y", what: "x"},
		{name: "mapfile", script: "mapfile -t a < <(printf '%s\\n' $y; printf '%s\\n' $y)", what: "a"},
		{name: "mapfile long record", script: "mapfile a <<< \"$z$y\"", what: "mapfile"},
		{name: "readarray", script: "readarray -t a < <(printf '%s\\n' $y; printf '%s\\n' $y)", what: "a"},
		{name: "default expansion", script: ": ${x:=$z$y}", what: "x"},
		{name: "eval", script: "eval 'x=$z$y'", what: "x"},
		{name: "function body", script: "f() { x=$z$y; }; f", what: "x"},
		{name: "command substitution", script: "x=$(for i in 1 2 3; do printf %s $y; done)", what: "command substitution"},
		{name: "unassigned command substitution", script: "echo \"$(for i in 1 2 3; do printf %s $y; done)\"", what: "command substitution"},
		{name: "pattern substitution", script: "echo ${y//?/$y}", what: "pattern substitution"},
		{name: "array pattern substitution", script: "a=($y); echo ${a[@]//?/$y}", what: "pattern substitution"},
		{name: "or list", script: "x=$z$y || echo handled", what: "x"},
		{name: "printf -v", script: "printf -v x '%120s' a", what: "printf"},
		{name: "printf width", script: "printf '%*s' 2000000000 x", what: "printf"},
		{name: "printf literal width", script: "printf '%2000000000s' x", what: "printf"},
		{name: "printf repeated fields", script: "printf '%60s%60s' a b", what: "printf"},
		{name: "positional doubling", script: "set -- $y; set -- \"$@\" \"$@\"", what: "argument list"},
		{name: "function argument doubling", script: "f() { f \"$@\" \"$@\"; }; f $y", what: "argument list"},
		{name: "for word list", script: "for x in $y $y; do :; done", what: "for word list"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 100})
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
			want := tc.what + ": value too large (limit 100 bytes), " + variableLimitHint
			if got := strings.Count(result.Stderr, want); got != 1 {
				t.Fatalf("Stderr = %q, want %q once", result.Stderr, want)
			}
		})
	}
}

func TestMaxVariableBytesLeavesValueUnchanged(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 100})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: fortyBytePrelude + "x=keep; trap 'echo trap x=$x' EXIT\nx=$y$y$y\n",
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
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 100})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: variableLimitPrelude +
			"( x=$z$y; echo no ); echo sub=$?\n" +
			"w=$(x=$z$y; echo no); echo cmd=$? w=$w\n" +
			"for i in 1 2 3; do printf %s $y; done | { read -r x; echo no; }; echo pipe=$?\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "sub=126\ncmd=126 w=\npipe=126\n"; got != want {
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
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 120})

	// y holds four zeros, so replacing each with 21 bytes gives exactly 120.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: fortyBytePrelude +
			"x=$y$y$y; z=$(for i in 1 2 3; do printf %s $y; done); read -r r <<< \"$x\"; printf -v p '%120s' a; w=${y//0/012345678901234567890}\n" +
			"a=(1 2 3 4 5 6 7); mapfile -t m < <(seq 7)\n" +
			"echo ${#x} ${#z} ${#r}; echo ${#p} ${#w}; echo ${#a[@]} ${#m[@]}\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "120 120 120\n120 120\n7 7\n"; got != want {
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

func TestMaxVariableBytesPrintfCommandHonorsLimit(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 120})

	// The registry printf command builds its output in memory too.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "env printf '%*s' 2000000000 x; echo rc=$?\nenv printf '%5s|\\n' x\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Stdout, "rc=126\n    x|\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if want := "printf: value too large (limit 120 bytes), " + variableLimitHint; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestMaxVariableBytesArgumentListBoundary(t *testing.T) {
	t.Parallel()
	// argv "set" "--" y y: 3 + 2 + 40 + 40 content bytes plus 16 per word.
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 149})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: fortyBytePrelude + "set -- $y $y; echo $#; printf '%*s|\\n' 3 x\nset -- $y $y x\necho unreachable\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "2\n  x|\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
