package pattern

import (
	"strings"
	"testing"
)

func TestMatchReviewRegressions(t *testing.T) {
	t.Parallel()
	ext := EntireString | ExtendedOperators
	longA := strings.Repeat("a", 1000)
	tests := []struct {
		name  string
		pat   string
		input string
		want  bool
	}{
		// [!a] negates in the recursive matcher used for !(...).
		{"negated class before negated group", `[!a]!(z)`, "ba", true},
		{"negated class rejects member", `[!a]!(z)`, "aa", false},
		// The second star of ** opens an extglob group.
		{"star before star group", `a**(a|b)!(z)`, "ab", true},
		// \é escapes the whole rune, not just its first byte.
		{"escaped multibyte rune", `\é`, "é", true},
		{"escaped multibyte rune in group", `\é!(z)`, "é", true},
		// Bracket escapes are literal characters, not regexp classes.
		{"escaped w in class", `[\w]`, "x", false},
		{"escaped w in class matches w", `[\w]`, "w", true},
		{"escaped d in class", `[\d]`, "5", false},
		{"escaped d in negated group class", `[\d]!(z)`, "5", false},
		{"escaped dash in group class", `[a\-z]!(q)`, "-", true},
		// Invalid UTF-8 forces the recursive matcher; it used to keep
		// full end-position slices per state and took ~20 seconds.
		{"many stars over invalid utf-8", `*a*a*a*a*a*a*a*a`, longA + "\xff", false},
		{"many stars over invalid utf-8 match", `*a*a*a*a*a*a*a*a`, "\xff" + longA, true},
		{"star group over invalid utf-8", `*(a)*a*a*a*a*a*a`, longA + "\xff", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Match(tc.pat, tc.input, ext)
			if err != nil {
				t.Fatalf("Match(%q) error: %v", tc.pat, err)
			}
			if got != tc.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", tc.pat, tc.input, got, tc.want)
			}
		})
	}
}
