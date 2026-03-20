package printfutil

import "testing"

func TestFormatShellQuoteAndZeroPadStrings(t *testing.T) {
	t.Parallel()

	result := Format("(%06s)\n[%q]\n", []string{"42", "a b"}, Options{})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if got, want := result.Output, "(000042)\n[a\\ b]\n"; got != want {
		t.Fatalf("Output = %q, want %q", got, want)
	}
}

func TestFormatQuotedCharUsesFirstByteForInvalidUnicode(t *testing.T) {
	t.Parallel()

	tooLarge := "'" + string([]byte{0xf4, 0x91, 0x84, 0x91})
	surrogate := "'" + string([]byte{0xed, 0xb0, 0x80})
	valid := "'μ"

	result := Format("%x\n%x\n%x\n", []string{tooLarge, surrogate, valid}, Options{})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if got, want := result.Output, "f4\ned\n3bc\n"; got != want {
		t.Fatalf("Output = %q, want %q", got, want)
	}
}
