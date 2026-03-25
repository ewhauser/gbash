package builtins_test

import (
	"context"
	"strings"
	"testing"
)

func TestUniqSupportsIgnoreCaseIsolated(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'Apple\\napple\\nBanana\\n' > /tmp/in.txt\nuniq --ignore-case -c /tmp/in.txt\nuniq -i /tmp/in.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "      2 Apple\n      1 Banana\nApple\nBanana\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestUniqSupportsGroupingAndAllRepeatedModes(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'a\\na\\nb\\nc\\nc\\n' > /tmp/in.txt\nuniq --all-repeated=separate /tmp/in.txt\nuniq --group=both /tmp/in.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "a\na\n\nc\nc\n\na\na\n\nb\n\nc\nc\n\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestUniqSupportsSkipFieldsSkipCharsAndOutputFile(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'a key-1\\nb key-1\\nc key-2\\n' > /tmp/in.txt\nuniq -f1 -s4 /tmp/in.txt /tmp/out.txt\ncat /tmp/out.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "a key-1\nc key-2\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestUniqSupportsZeroTerminatedAndLegacySyntax(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'a\\0a\\0b\\0' | uniq -z\nprintf 'x same\\ny same\\n' > /tmp/legacy.txt\n_POSIX2_VERSION=199209 uniq +1 /tmp/legacy.txt\nuniq -1 /tmp/legacy.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "a\x00b\x00x same\nx same\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestUniqRejectsInvalidGroupCombinations(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "uniq --group -c\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got := result.Stderr; !strings.Contains(got, "--group is mutually exclusive") {
		t.Fatalf("Stderr = %q, want group conflict error", got)
	}
}

func TestUniqUsesGNUQuotedInvalidMethodDiagnostics(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "uniq --all-repeated=badoption\nuniq --group=badoption\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got := result.Stderr; !strings.Contains(got, "uniq: invalid argument 'badoption' for '--all-repeated'") {
		t.Fatalf("Stderr = %q, want GNU-style all-repeated diagnostic", got)
	}
	if got := result.Stderr; strings.Contains(got, "\"badoption\"") {
		t.Fatalf("Stderr = %q, want single-quoted operand", got)
	}
}

func TestUniqAcceptsOverflowingNumericOptions(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'a\\na\\n\\b' | uniq -d -u -w18446744073709551616\nprintf 'a\\na\\n\\b' | uniq -d -u -w18446744073709551617\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got := result.Stdout; got != "" {
		t.Fatalf("Stdout = %q, want empty", got)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}
