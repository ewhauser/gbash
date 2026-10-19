package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestSignalTrapsFromBackgroundJobs(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// Background kills queue traps on the shell while it runs; under -race
	// this used to report data races on the pending trap queue.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "trap 'echo t' USR1\nfor i in {1..20}; do kill -USR1 $$ & done\nwait\nsleep 0\necho done\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.HasSuffix(result.Stdout, "done\n") {
		t.Fatalf("Stdout = %q, want it to end with done", result.Stdout)
	}
	if got := strings.Count(result.Stdout, "t\n"); got == 0 || got > 20 {
		t.Fatalf("trap ran %d times, want 1 to 20; stdout=%q", got, result.Stdout)
	}
}
