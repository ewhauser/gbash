package interp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/contrib/awk/goawk/interp"
	"github.com/ewhauser/gbash/contrib/awk/goawk/parser"
)

const (
	testMaxStringBytes = 4096
	testMaxArrayBytes  = 64 << 10

	stringLimitError = "string exceeds the 4096-byte memory limit"
	arrayLimitError  = "arrays exceed the 65536-byte memory limit"
)

func execWithLimits(t *testing.T, src, stdin string, maxString int, maxArray int64) (string, error) {
	t.Helper()

	prog, err := parser.ParseProgram([]byte(src), nil)
	if err != nil {
		t.Fatalf("ParseProgram() error = %v", err)
	}
	var out bytes.Buffer
	_, err = interp.ExecProgram(prog, &interp.Config{
		Stdin:          strings.NewReader(stdin),
		Output:         &out,
		Error:          &out,
		Environ:        []string{},
		MaxStringBytes: maxString,
		MaxArrayBytes:  maxArray,
	})
	return out.String(), err
}

func TestLimitsStopRunawayPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		stdin string
		want  string
	}{
		{"concat", `BEGIN { s = "x"; while (1) s = s s }`, "", stringLimitError},
		{"multi concat", `BEGIN { s = "x"; while (1) s = s "-" s }`, "", stringLimitError},
		{"gsub amplification", `BEGIN { s = "x"; while (1) gsub(/x/, "&&", s) }`, "", stringLimitError},
		{"gsub huge replacement", `BEGIN { s = "x"; for (i = 0; i < 7; i++) s = s s; r = s; gsub(/x/, "\\&", r); gsub(/x/, r, s) }`, "", stringLimitError},
		{"sub on field", `{ while (1) sub(/x+/, "&&") }`, "x\n", stringLimitError},
		{"sprintf literal width", `BEGIN { s = sprintf("%5000d", 1) }`, "", stringLimitError},
		{"sprintf precision", `BEGIN { s = sprintf("%.5000f", 1) }`, "", stringLimitError},
		{"printf star width", `BEGIN { printf "%*d", 5000, 1 }`, "", stringLimitError},
		{"record rebuilt with long OFS", `BEGIN { OFS = sprintf("%1000s", ""); $0 = "a"; NF = 10 }`, "", stringLimitError},
		{"field assignment rebuild", `BEGIN { s = sprintf("%1000s", ""); for (i = 1; i <= 10; i++) $i = s }`, "", stringLimitError},
		{"multi-dimensional subscript", `BEGIN { s = sprintf("%1000s", ""); a[s, s, s, s, s] = 1 }`, "", stringLimitError},
		{"auto-created elements", `BEGIN { while (1) a[n++] }`, "", arrayLimitError},
		{"assigned elements", `BEGIN { while (1) a[n++] = n }`, "", arrayLimitError},
		{"incremented elements", `BEGIN { while (1) a[n++]++ }`, "", arrayLimitError},
		{"aug-assigned elements", `BEGIN { while (1) a[n++] += 2 }`, "", arrayLimitError},
		{"shared large values", `BEGIN { s = sprintf("%4000s", ""); while (1) a[n++] = s }`, "", arrayLimitError},
		{"local array", `function f(arr) { while (1) arr[n++] = n } BEGIN { f() }`, "", arrayLimitError},
		{"getline into array", `BEGIN { while ((getline a[n++]) > 0) {} }`, strings.Repeat("line\n", 2000), arrayLimitError},
		{"split by char", `BEGIN { s = sprintf("%4000s", ""); gsub(/ /, "x", s); split(s, a, "") }`, "", arrayLimitError},
		{"split by single char", `BEGIN { s = sprintf("%4000s", ""); gsub(/ /, ",", s); split(s, a, ",") }`, "", arrayLimitError},
		{"split by whitespace", `BEGIN { s = sprintf("%4000s", ""); gsub(/  /, "x ", s); split(s, a) }`, "", arrayLimitError},
		{"split by regex", `BEGIN { s = sprintf("%4000s", ""); gsub(/ /, "x", s); split(s, a, /x/) }`, "", arrayLimitError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := execWithLimits(t, tc.src, tc.stdin, testMaxStringBytes, testMaxArrayBytes)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q; output=%q", err, tc.want, out)
			}
		})
	}
}

func TestLimitsAllowProgramsWithinBounds(t *testing.T) {
	t.Parallel()

	// Each pass holds about 40 KiB of array values and then frees them, so
	// the passes add up to far more than the array budget unless freed
	// elements are credited back.
	src := `
function fill(n, local) { for (i = 0; i < 10; i++) local[i] = s i n; return length(local) }
BEGIN {
	s = sprintf("%4090s", "")
	t = s "123456"
	for (pass = 0; pass < 20; pass++) {
		for (i = 0; i < 10; i++) a[i] = s i pass
		delete a[0]
		delete a
		total += fill(pass)
		total += split(s, b, "q")
		c[pass]++
		c[pass] += 1
		delete c[pass]
	}
	print length(t), total
}`
	out, err := execWithLimits(t, src, "", testMaxStringBytes, testMaxArrayBytes)
	if err != nil {
		t.Fatalf("error = %v; output=%q", err, out)
	}
	if want := "4096 220\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestLimitsDisabledByDefault(t *testing.T) {
	t.Parallel()

	out, err := execWithLimits(t, `BEGIN { s = "x"; for (i = 0; i < 16; i++) s = s s; for (i = 0; i < 20000; i++) a[i] = s; print length(s), length(a) }`, "", 0, 0)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if want := "65536 20000\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}
