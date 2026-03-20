package printfutil

import (
	"runtime"
	"testing"
)

func TestFormatShellQuoteAndZeroPadStrings(t *testing.T) {
	t.Parallel()

	result := Format("(%06s)\n[%q]\n", []string{"42", "a b"}, Options{})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	want := "(000042)\n[a\\ b]\n"
	if runtime.GOOS == "linux" {
		want = "(    42)\n[a\\ b]\n"
	}
	if got := result.Output; got != want {
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

func TestFormatOverflowDiagnosticsMatchPlatformOracle(t *testing.T) {
	t.Parallel()

	result := Format("%d\n", []string{"18446744073709551616"}, Options{})
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	want := "18446744073709551616: Result too large"
	if runtime.GOOS == "linux" {
		want = "18446744073709551616: Numerical result out of range"
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0] != want {
		t.Fatalf("Diagnostics = %v, want [%q]", result.Diagnostics, want)
	}
}
