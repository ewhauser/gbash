package runtime

import "testing"

func TestShellClassicTestMatchesSharedParenthesizedLiteralCases(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := mustExecSession(t, session,
		"test 0 -eq 0 -a '(' -f ')'\n"+
			"printf 'test-f=%s\\n' \"$?\"\n"+
			"test 0 -eq 0 -a '(' -t ')'\n"+
			"printf 'test-t=%s\\n' \"$?\"\n"+
			"test 0 -eq 0 -a '(' ! ')'\n"+
			"printf 'test-bang=%s\\n' \"$?\"\n"+
			"[ 0 -eq 0 -a '(' -f ')' ]\n"+
			"printf 'bracket-f=%s\\n' \"$?\"\n"+
			"[ 0 -eq 0 -a '(' -t ')' ]\n"+
			"printf 'bracket-t=%s\\n' \"$?\"\n"+
			"[ 0 -eq 0 -a '(' ! ')' ]\n"+
			"printf 'bracket-bang=%s\\n' \"$?\"\n",
	)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, ""+
		"test-f=0\n"+
		"test-t=0\n"+
		"test-bang=0\n"+
		"bracket-f=0\n"+
		"bracket-t=0\n"+
		"bracket-bang=0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if result.Stderr != "" {
		t.Fatalf("Stderr = %q, want empty", result.Stderr)
	}
}
