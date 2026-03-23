package printfutil

import (
	"runtime"
	"testing"
	"time"
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

func TestFormatSupportsUppercaseEAndAlternateHexZero(t *testing.T) {
	t.Parallel()

	result := Format("[%E][%#x][%#x][%#X][%#X]\n", []string{"3.14", "0", "42", "0", "42"}, Options{})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if got, want := result.Output, "[3.140000E+00][0][0x2a][0][0X2A]\n"; got != want {
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
	want := "f4\ned\n3bc\n"
	if runtime.GOOS == "linux" {
		want = "111111\ned\n3bc\n"
	}
	if got := result.Output; got != want {
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

func TestFormatTimeSentinelsAndExtendedDirectives(t *testing.T) {
	t.Parallel()

	now := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	start := time.Date(2019, time.May, 15, 17, 3, 19, 0, time.UTC)
	result := Format("%(%F %T %z %s)T\n%(%F)T\n", []string{"-1", "-2"}, Options{
		LookupEnv: func(name string) (string, bool) {
			if name == "TZ" {
				return "UTC", true
			}
			return "", false
		},
		Now:       func() time.Time { return now },
		StartTime: start,
	})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if got, want := result.Output, "2020-01-02 03:04:05 +0000 1577934245\n2019-05-15\n"; got != want {
		t.Fatalf("Output = %q, want %q", got, want)
	}
}

func TestFormatRejectsInvalidModifierBeforeVerb(t *testing.T) {
	t.Parallel()

	result := Format("%Zs\n", []string{"x"}, Options{})
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0] != "`Z': invalid format character" {
		t.Fatalf("Diagnostics = %v, want invalid-format diagnostic", result.Diagnostics)
	}
}

func TestFormatGNUSupportsEscapeE(t *testing.T) {
	t.Parallel()

	result := Format("\\e", nil, Options{Dialect: DialectGNU})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if got, want := result.Output, "\x1b"; got != want {
		t.Fatalf("Output = %q, want %q", got, want)
	}
}

func TestFormatGNURejectsMalformedOuterEscapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "hex", format: "A\\xZ", want: "A"},
		{name: "short-unicode", format: "A\\uabc", want: "A"},
		{name: "short-wide-unicode", format: "A\\U1234", want: "A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Format(tt.format, nil, Options{Dialect: DialectGNU})
			if result.ExitCode != 1 {
				t.Fatalf("ExitCode = %d, want 1; diagnostics=%v", result.ExitCode, result.Diagnostics)
			}
			if got := result.Output; got != tt.want {
				t.Fatalf("Output = %q, want %q", got, tt.want)
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0] != "missing hexadecimal number in escape" {
				t.Fatalf("Diagnostics = %v, want missing-hex diagnostic", result.Diagnostics)
			}
		})
	}
}

func TestFormatGNUPercentBStopsOnMalformedEscape(t *testing.T) {
	t.Parallel()

	result := Format("%b|%s", []string{"A\\xZ", "B"}, Options{Dialect: DialectGNU})
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if got, want := result.Output, "A"; got != want {
		t.Fatalf("Output = %q, want %q", got, want)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0] != "missing hexadecimal number in escape" {
		t.Fatalf("Diagnostics = %v, want missing-hex diagnostic", result.Diagnostics)
	}
}

func TestFormatGNUQuoteMatchesCoreutils(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "empty", arg: "", want: "''"},
		{name: "raw", arg: "abc", want: "abc"},
		{name: "space", arg: "a b", want: "'a b'"},
		{name: "single-quote", arg: "'", want: "\"'\""},
		{name: "double-quote", arg: "\"", want: "'\"'"},
		{name: "newline", arg: "a\n", want: "'a'$'\\n'"},
		{name: "tab", arg: "a\tb", want: "'a'$'\\t''b'"},
		{name: "control", arg: string([]byte{0x01}), want: "''$'\\001'"},
		{name: "control-quote-control", arg: string([]byte{0x01, '\'', 0x01}), want: "''$'\\001'\\'''$'\\001'"},
		{name: "leading-tilde", arg: "~foo", want: "'~foo'"},
		{name: "leading-hash", arg: "#foo", want: "'#foo'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Format("%q", []string{tt.arg}, Options{Dialect: DialectGNU})
			if result.ExitCode != 0 {
				t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
			}
			if got := result.Output; got != tt.want {
				t.Fatalf("Output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatGNURejectsShellOnlyConversionsAndFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "time", format: "%(%F)T", want: "%(: invalid conversion specification"},
		{name: "quote-flag-s", format: "%'s", want: "%'s: invalid conversion specification"},
		{name: "quote-flag-c", format: "%'c", want: "%'c: invalid conversion specification"},
		{name: "quote-flag-x", format: "%'x", want: "%'x: invalid conversion specification"},
		{name: "quote-flag-o", format: "%'o", want: "%'o: invalid conversion specification"},
		{name: "width-q", format: "%7q", want: "%7q: invalid conversion specification"},
		{name: "width-b", format: "%7b", want: "%7b: invalid conversion specification"},
		{name: "length-q", format: "%lq", want: "%lq: invalid conversion specification"},
		{name: "length-llq", format: "%llq", want: "%llq: invalid conversion specification"},
		{name: "length-b", format: "%lb", want: "%lb: invalid conversion specification"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Format(tt.format, []string{"0"}, Options{Dialect: DialectGNU})
			if result.ExitCode != 1 {
				t.Fatalf("ExitCode = %d, want 1; diagnostics=%v", result.ExitCode, result.Diagnostics)
			}
			if got := result.Output; got != "" {
				t.Fatalf("Output = %q, want empty", got)
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0] != tt.want {
				t.Fatalf("Diagnostics = %v, want [%q]", result.Diagnostics, tt.want)
			}
		})
	}
}

func TestFormatGNUAllowsQuoteFlagAndLengthModifiersWhereCoreutilsDoes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		arg    string
		want   string
	}{
		{name: "quote-flag-d", format: "%'d", arg: "1000", want: "1000"},
		{name: "quote-flag-u", format: "%'u", arg: "1000", want: "1000"},
		{name: "quote-flag-f", format: "%'f", arg: "1000", want: "1000.000000"},
		{name: "length-s", format: "%ls", arg: "x", want: "x"},
		{name: "length-d", format: "%ld", arg: "10", want: "10"},
		{name: "length-f", format: "%Lf", arg: "10", want: "10.000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Format(tt.format, []string{tt.arg}, Options{Dialect: DialectGNU})
			if result.ExitCode != 0 {
				t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
			}
			if got := result.Output; got != tt.want {
				t.Fatalf("Output = %q, want %q", got, tt.want)
			}
			if len(result.Diagnostics) != 0 {
				t.Fatalf("Diagnostics = %v, want empty", result.Diagnostics)
			}
		})
	}
}

func TestFormatGNUWarnsOnExcessArgsWithoutConversions(t *testing.T) {
	t.Parallel()

	result := Format("plain", []string{"extra", "more"}, Options{Dialect: DialectGNU})
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; diagnostics=%v", result.ExitCode, result.Diagnostics)
	}
	if got, want := result.Output, "plain"; got != want {
		t.Fatalf("Output = %q, want %q", got, want)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("Diagnostics = %v, want empty", result.Diagnostics)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "warning: ignoring excess arguments, starting with 'extra'" {
		t.Fatalf("Warnings = %v, want excess-args warning", result.Warnings)
	}
}
