package builtins_test

import "testing"

func TestGzipQuietSuppressesMissingInputDiagnostics(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	result := mustExecSession(t, session, "missing=\ngzip -cdfq -- \"$missing\"\n")
	if got, want := result.ExitCode, 1; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}
