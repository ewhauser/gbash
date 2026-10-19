package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

const totalVariableLimitMessage = "total variable size too large (limit "

func TestMaxTotalVariableBytesDefaultStopsManyLargeVariables(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// Every value fits under MaxVariableBytes, but together they used to
	// reach about 7 GB.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: `x=$(printf "%08000000d" 0); for i in {1..300}; do declare "v$i=$x$i"; done; echo unreachable` + "\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if strings.Contains(result.Stdout, "unreachable") {
		t.Fatalf("Stdout = %q, want the shell to stop", result.Stdout)
	}
	if want := "v32: " + totalVariableLimitMessage + "268435456 bytes), increase policy.Limits.MaxTotalVariableBytes"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestMaxTotalVariableBytesReleasesUnsetAndLocalValues(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxVariableBytes: 1000, MaxTotalVariableBytes: 3500})

	// Values that are unset, overwritten, or local to a returned function
	// stop counting toward the total.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: `x=$(printf "%0900d" 0)
for i in 1 2 3 4 5; do v=$x; unset v; done
f() { local a=$x b=$x; }
for i in 1 2 3 4 5; do f; done
y=$x; y=$x; ( z=$x; echo "sub ${#z}" )
echo "ok ${#y}"
a=$x
b=$x
echo unreachable
`,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "sub 900\nok 900\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if want := "b: " + totalVariableLimitMessage + "3500 bytes)"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}
