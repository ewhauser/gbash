package pattern

import (
	"regexp"
	"strings"
	"testing"
)

// An extglob group without its closing parenthesis used to produce an
// invalid regular expression (leaking the lexer's NUL end marker), and
// callers panicked compiling it. Bash matches such a group, and everything
// after it, literally.
func TestUnbalancedExtGlobIsLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pat          string
		mustMatch    []string
		mustNotMatch []string
	}{
		{pat: "*(", mustMatch: []string{"*("}, mustNotMatch: []string{"", "(", "x("}},
		{pat: "@(a|", mustMatch: []string{"@(a|"}, mustNotMatch: []string{"a", ""}},
		{pat: "+(a", mustMatch: []string{"+(a"}, mustNotMatch: []string{"a", "aa"}},
		{pat: "*(b", mustMatch: []string{"*(b"}, mustNotMatch: []string{"xyz(b"}},
		{pat: "?(a", mustMatch: []string{"?(a"}, mustNotMatch: []string{"z(a"}},
		{pat: "*(a?b", mustMatch: []string{"*(a?b"}, mustNotMatch: []string{"*(axb"}},
		{pat: "*(a[bc]", mustMatch: []string{"*(a[bc]"}, mustNotMatch: []string{"*(ab"}},
		{pat: `*(a\x`, mustMatch: []string{`*(a\x`}, mustNotMatch: []string{"*(ax"}},
		{pat: "@(*(b|c)", mustMatch: []string{"@(*(b|c)"}, mustNotMatch: []string{"b", "@(b"}},
		{pat: "*(a)*(b", mustMatch: []string{"aaa*(b", "*(b"}, mustNotMatch: []string{"aaab(b", "aaab"}},
		{pat: "x*(", mustMatch: []string{"x*("}, mustNotMatch: []string{"x", "xyz("}},
	}
	for _, tc := range tests {
		t.Run(tc.pat, func(t *testing.T) {
			t.Parallel()
			expr, err := Regexp(tc.pat, EntireString|ExtendedOperators)
			if err != nil {
				t.Fatalf("Regexp(%q) error = %v", tc.pat, err)
			}
			if strings.ContainsRune(expr, 0) {
				t.Fatalf("Regexp(%q) = %q, contains a NUL byte", tc.pat, expr)
			}
			rx, err := regexp.Compile(expr)
			if err != nil {
				t.Fatalf("Regexp(%q) = %q, which does not compile: %v", tc.pat, expr, err)
			}
			matcher, err := ExtendedPatternMatcher(tc.pat, EntireString|ExtendedOperators)
			if err != nil {
				t.Fatalf("ExtendedPatternMatcher(%q) error = %v", tc.pat, err)
			}
			for _, s := range tc.mustMatch {
				if !rx.MatchString(s) || !matcher(s) {
					t.Errorf("%q should match %q (regexp %q)", tc.pat, s, expr)
				}
			}
			for _, s := range tc.mustNotMatch {
				if rx.MatchString(s) || matcher(s) {
					t.Errorf("%q should not match %q (regexp %q)", tc.pat, s, expr)
				}
			}
		})
	}
}

// Negated groups are matched by a separate parser, which must treat an
// unterminated group the same way.
func TestUnbalancedExtGlobAfterNegation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pat          string
		mustMatch    []string
		mustNotMatch []string
	}{
		{pat: "!(a", mustMatch: []string{"!(a"}, mustNotMatch: []string{"b", ""}},
		{pat: "!(a)*(b", mustMatch: []string{"c*(b", "*(b"}, mustNotMatch: []string{"a*(b", "cb"}},
	}
	for _, tc := range tests {
		matcher, err := ExtendedPatternMatcher(tc.pat, EntireString|ExtendedOperators)
		if err != nil {
			t.Fatalf("ExtendedPatternMatcher(%q) error = %v", tc.pat, err)
		}
		for _, s := range tc.mustMatch {
			if !matcher(s) {
				t.Errorf("%q should match %q", tc.pat, s)
			}
		}
		for _, s := range tc.mustNotMatch {
			if matcher(s) {
				t.Errorf("%q should not match %q", tc.pat, s)
			}
		}
	}
}
