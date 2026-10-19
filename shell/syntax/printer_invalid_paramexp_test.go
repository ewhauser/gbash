package syntax

import (
	"strings"
	"testing"
)

// The parser keeps a bad substitution such as ${%} for bash's runtime error,
// with a nil Param. Minify used to dereference that Param (found by
// FuzzParsePrint), and the default printer dropped the operator, printing
// "${}" (for example in declare -f output).
func TestPrintInvalidParamExpKeepsSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src  string
		want string
	}{
		{src: "echo ${%}", want: "echo ${%}"},
		{src: `echo "${%}"`, want: `echo "${%}"`},
		{src: "echo ${%}x", want: "echo ${%}x"},
		{src: "f() { echo ${%}; }", want: "f() { echo ${%}; }"},
	}
	for _, tc := range tests {
		for _, opts := range [][]PrinterOption{nil, {Minify(true)}, {SingleLine(true)}} {
			f, err := NewParser().Parse(strings.NewReader(tc.src), "")
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tc.src, err)
			}
			Simplify(f)
			var sb strings.Builder
			if err := NewPrinter(opts...).Print(&sb, f); err != nil {
				t.Fatalf("Print(%q) error = %v", tc.src, err)
			}
			got := strings.TrimSpace(sb.String())
			if !strings.Contains(got, "${%}") {
				t.Fatalf("Print(%q) with %d options = %q, want it to keep ${%%}", tc.src, len(opts), got)
			}
			if len(opts) == 0 && got != tc.want {
				t.Fatalf("Print(%q) = %q, want %q", tc.src, got, tc.want)
			}
		}
	}
}
