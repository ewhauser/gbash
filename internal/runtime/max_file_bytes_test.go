package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

func TestMaxFileBytesEnforcedForHelperStdinReads(t *testing.T) {
	rt := newRuntimeWithLimits(t, policy.Limits{MaxFileBytes: 3})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "grep a\n",
		Stdin:  strings.NewReader("abcd"),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.ExitCode, 1; got != want {
		t.Fatalf("ExitCode = %d, want %d", got, want)
	}
	if got, want := result.Stderr, "input exceeds maximum file size of 3 bytes\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestMaxFileBytesEnforcedForCatFileReads(t *testing.T) {
	rt := newRuntimeWithLimits(t, policy.Limits{MaxFileBytes: 3})
	session, err := rt.NewSession(context.Background())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	writeSessionFile(t, session, "/tmp/input.txt", []byte("abcd"))

	result, err := session.Exec(context.Background(), &ExecutionRequest{
		Script: "cat /tmp/input.txt\n",
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if got, want := result.ExitCode, 1; got != want {
		t.Fatalf("ExitCode = %d, want %d", got, want)
	}
	if got, want := result.Stderr, "cat: /tmp/input.txt: input exceeds maximum file size of 3 bytes\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestMaxFileBytesEnforcedForBashStdinReads(t *testing.T) {
	rt := newRuntimeWithLimits(t, policy.Limits{MaxFileBytes: 3})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "bash\n",
		Stdin:  strings.NewReader("echo hi\n"),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.ExitCode, 1; got != want {
		t.Fatalf("ExitCode = %d, want %d", got, want)
	}
	if got, want := result.Stderr, "input exceeds maximum file size of 3 bytes\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestMaxFileBytesEnforcedForBashScriptFiles(t *testing.T) {
	rt := newRuntimeWithLimits(t, policy.Limits{MaxFileBytes: 3})
	session, err := rt.NewSession(context.Background())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	writeSessionFile(t, session, "/tmp/script.sh", []byte("echo hi\n"))

	result, err := session.Exec(context.Background(), &ExecutionRequest{
		Script: "bash /tmp/script.sh\n",
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if got, want := result.ExitCode, 1; got != want {
		t.Fatalf("ExitCode = %d, want %d", got, want)
	}
	if got, want := result.Stderr, "bash: /tmp/script.sh: input exceeds maximum file size of 3 bytes\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}
