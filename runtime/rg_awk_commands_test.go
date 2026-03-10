package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestRGSearchesRecursivelyWithGlobFilter(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "mkdir -p dir/sub\nprintf 'needle\\n' > dir/a.txt\nprintf 'needle\\n' > dir/sub/b.md\nprintf 'needle\\n' > dir/.hidden.txt\nrg -n -g '*.txt' needle dir\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "/home/agent/dir/a.txt:1:needle\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestRGHiddenModeIncludesDotfiles(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "mkdir -p dir\nprintf 'needle\\n' > dir/.hidden.txt\nrg --hidden -n needle dir\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "/home/agent/dir/.hidden.txt:1:needle\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestAWKSupportsProgramFilesFieldSeparatorsAndVars(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'BEGIN { print prefix }\\n{ print $2 }\\n' > /tmp/prog.awk\nprintf 'a,b\\nc,d\\n' > /tmp/in.csv\nawk -F, -v prefix=rows -f /tmp/prog.awk /tmp/in.csv\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "rows\nb\nd\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestAWKDisablesExec(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "awk 'BEGIN { system(\"echo nope\") }'\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("ExitCode = %d, want non-zero", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "NoExec") && !strings.Contains(result.Stderr, "can't") {
		t.Fatalf("Stderr = %q, want sandbox execution denial", result.Stderr)
	}
}
