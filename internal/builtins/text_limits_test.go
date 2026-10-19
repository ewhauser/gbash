package builtins_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

// execWithDeadline runs script in a fresh session and fails the test if it
// does not finish within a generous deadline, so regressions that loop or
// allocate without bound surface as failures instead of hangs.
func execWithDeadline(t *testing.T, cfg *Config, script string) *ExecutionResult {
	t.Helper()
	if cfg == nil {
		cfg = &Config{}
	}
	session := newSession(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := session.Exec(ctx, &ExecutionRequest{Script: script})
	if err != nil {
		t.Fatalf("Session.Exec() error = %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("script did not finish before the deadline: %q", script)
	}
	return result
}

func assertExitStderr(t *testing.T, result *ExecutionResult, wantCode int, wantStderr string) {
	t.Helper()
	if result.ExitCode != wantCode {
		t.Fatalf("ExitCode = %d, want %d; stdout=%q stderr=%q", result.ExitCode, wantCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, wantStderr) {
		t.Fatalf("Stderr = %q, want substring %q", result.Stderr, wantStderr)
	}
}

const fieldTooLargeMessage = "field width too large (limit 33554432 bytes)"

func TestODRejectsOversizedWidth(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "echo abc | od -An -tx1 -w200000000000000\n")
	assertExitStderr(t, result, 1, "od: "+fieldTooLargeMessage)
	if result.Stdout != "" {
		t.Fatalf("Stdout = %q, want empty", result.Stdout)
	}
}

func TestODReusesPaddingBufferAcrossLines(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "printf 'abcdefghij' | od -An -tx1 -w4\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, " 61 62 63 64\n 65 66 67 68\n 69 6a\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestTRHighByteRangeTerminates(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "printf 'a\\200b\\377c' | tr -d '\\200-\\377'\nprintf '\\376\\377' | tr '\\376-\\377' 'yz'\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "abcyz"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestTRRejectsOversizedRepeatCounts(t *testing.T) {
	t.Parallel()
	for _, script := range []string{
		"echo abc | tr a '[x*9223372036854775807]'\n",
		"echo abc | tr '[a*4294967296]' x\n",
	} {
		result := execWithDeadline(t, nil, script)
		assertExitStderr(t, result, 1, "tr: string too long (limit 33554432 bytes)")
	}
	result := execWithDeadline(t, nil, "echo abc | tr a '[x*5]'\n")
	if got, want := result.Stdout, "xbc\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q", result.ExitCode, got, want)
	}
}
