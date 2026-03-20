package builtins_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ewhauser/gbash/internal/builtins"
)

type errWriter struct {
	err error
}

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestPrintfSupportsBashNumericCharConstants(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session,
		"single=\"'A\"\n"+
			"double='\"B'\n"+
			"printf '%d|%i|%o|%u|%x|%X|%.1f|%g\\n' \"$single\" \"$single\" \"$single\" \"$single\" \"$single\" \"$single\" \"$single\" \"$double\"\n",
	)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "65|65|101|65|41|41|65.0|66\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestPrintfCharacterFormatUsesFirstCharacter(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session,
		"quoted=\"'B\"\n"+
			"printf '%c%c%c%c' A 65 \"$quoted\" '' | od -An -tx1 -v\n",
	)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, " 41 36 27 00\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestPrintfWriteFailureReturnsExitStatusOne(t *testing.T) {
	t.Parallel()

	cmd := builtins.NewPrintf()
	err := cmd.Run(context.Background(), &builtins.Invocation{
		Args:   []string{"%s", "hi"},
		Stdout: errWriter{err: errors.New("sink failed")},
	})
	var exitErr *builtins.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %T %v, want *ExitError", err, err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("ExitError.Code = %d, want 1", exitErr.Code)
	}
}
