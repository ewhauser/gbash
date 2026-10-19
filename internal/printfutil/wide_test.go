package printfutil

import (
	"strings"
	"testing"
)

func TestFormatRejectsPrecisionPastOutputLimit(t *testing.T) {
	t.Parallel()

	for _, dialect := range []Dialect{DialectShell, DialectGNU} {
		for _, format := range []string{"%.300000000f", "%.40000000d", "%030000000d"} {
			result := Format(format, []string{"1"}, Options{Dialect: dialect, MaxOutputBytes: 1 << 20})
			if !result.LimitExceeded || result.Output != "" {
				t.Fatalf("Format(%q) dialect %v = %+v, want LimitExceeded with no output", format, dialect, result)
			}
		}
	}
}

func TestFormatPadsPastFmtWidthCeiling(t *testing.T) {
	t.Parallel()

	const n = 1_500_000
	cases := []struct {
		format string
		arg    string
		want   string
	}{
		{"%0*d", "-5", "-" + strings.Repeat("0", n-2) + "5"},
		{"%*d", "7", strings.Repeat(" ", n-1) + "7"},
		{"%-*d|", "7", "7" + strings.Repeat(" ", n-1) + "|"},
		{"%.*f", "1.25", "1.25" + strings.Repeat("0", n-2)},
		{"%+.*d", "3", "+" + strings.Repeat("0", n-1) + "3"},
		{"%#.*x", "255", "0x" + strings.Repeat("0", n-2) + "ff"},
		{"%#.*o", "8", strings.Repeat("0", n-2) + "10"},
		{"%0*.3f", "-1.5", "-" + strings.Repeat("0", n-6) + "1.500"},
	}
	for _, dialect := range []Dialect{DialectShell, DialectGNU} {
		for _, tc := range cases {
			result := Format(tc.format, []string{"1500000", tc.arg}, Options{Dialect: dialect, MaxOutputBytes: 32 << 20})
			if result.LimitExceeded || result.ExitCode != 0 {
				t.Fatalf("Format(%q, %q) dialect %v: exit=%d limit=%v diags=%v", tc.format, tc.arg, dialect, result.ExitCode, result.LimitExceeded, result.Diagnostics)
			}
			if strings.Contains(result.Output, "%!") || result.Output != tc.want {
				t.Fatalf("Format(%q, %q) dialect %v = %d bytes starting %q, want %d bytes starting %q", tc.format, tc.arg, dialect, len(result.Output), head(result.Output), len(tc.want), head(tc.want))
			}
		}
	}
}

func head(s string) string {
	return s[:min(len(s), 12)] + "..." + s[max(0, len(s)-12):]
}
