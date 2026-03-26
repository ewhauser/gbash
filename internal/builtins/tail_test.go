package builtins_test

import (
	"context"
	"testing"
)

func TestTailPidWithDeadProcessExitsImmediately(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'hello\\n' >/tmp/in.txt\n" +
			"tail -n 0 -f --pid=2147483647 /tmp/in.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got := result.Stdout; got != "" {
		t.Fatalf("Stdout = %q, want empty output", got)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty stderr", got)
	}
}
