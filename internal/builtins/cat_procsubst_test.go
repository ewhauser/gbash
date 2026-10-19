package builtins_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCatReadsProcessSubstitutionWithHostStdout(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// With stdout on a host file, cat's same-file check used to open the
	// process substitution to compare it, consuming it before cat read it.
	out, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer func() { _ = out.Close() }()

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "cat <(echo hi)\n",
		Stdout: out,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	written, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(written), "hi\n"; got != want {
		t.Fatalf("stdout file = %q, want %q", got, want)
	}
}
