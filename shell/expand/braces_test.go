// Copyright (c) 2018, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package expand

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/ewhauser/gbash/shell/syntax"
)

func litWord(s string) *syntax.Word {
	return &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: s}}}
}

func litWords(strs ...string) []*syntax.Word {
	l := make([]*syntax.Word, 0, len(strs))
	for _, s := range strs {
		l = append(l, litWord(s))
	}
	return l
}

var braceTests = []struct {
	src  string
	want []string
}{
	{
		src:  "a{b",
		want: []string{"a{b"},
	},
	{
		src:  "a}b",
		want: []string{"a}b"},
	},
	{
		src:  "{a,b{c,d}",
		want: []string{"{a,bc", "{a,bd"},
	},
	{
		src:  "{a{b",
		want: []string{"{a{b"},
	},
	{
		src:  "a{}",
		want: []string{"a{}"},
	},
	{
		src:  "a{b}",
		want: []string{"a{b}"},
	},
	{
		src:  "a{b,c}",
		want: []string{"ab", "ac"},
	},
	{
		src:  "a{à,世界}",
		want: []string{"aà", "a世界"},
	},
	{
		src:  "a{b,c}d{e,f}g",
		want: []string{"abdeg", "abdfg", "acdeg", "acdfg"},
	},
	{
		src:  "a{b{x,y},c}d",
		want: []string{"abxd", "abyd", "acd"},
	},
	{
		src:  "a{1,2,3,4,5}",
		want: []string{"a1", "a2", "a3", "a4", "a5"},
	},
	{
		src:  "-{a,b,1..3}-",
		want: []string{"-a-", "-b-", "-1..3-"},
	},
	{
		src:  "a{1..",
		want: []string{"a{1.."},
	},
	{
		src:  "a{1..4",
		want: []string{"a{1..4"},
	},
	{
		src:  "a{1.4}",
		want: []string{"a{1.4}"},
	},
	{
		src:  "{a,b}{1..4",
		want: []string{"a{1..4", "b{1..4"},
	},
	{
		src:  "a{1..4}",
		want: []string{"a1", "a2", "a3", "a4"},
	},
	{
		src:  "a{1..2}b{4..5}c",
		want: []string{"a1b4c", "a1b5c", "a2b4c", "a2b5c"},
	},
	{
		src:  "a{1..f}",
		want: []string{"a{1..f}"},
	},
	{
		src:  "a{c..f}",
		want: []string{"ac", "ad", "ae", "af"},
	},
	{
		src:  "a{H..K}",
		want: []string{"aH", "aI", "aJ", "aK"},
	},
	{
		src:  "a{-..f}",
		want: []string{"a{-..f}"},
	},
	{
		src:  "a{3..-}",
		want: []string{"a{3..-}"},
	},
	{
		src:  "a{1..10..3}",
		want: []string{"a1", "a4", "a7", "a10"},
	},
	{
		src:  "a{1..8..-3}",
		want: []string{"a1", "a4", "a7"},
	},
	{
		src:  "a{1..4..0}",
		want: []string{"a1", "a2", "a3", "a4"},
	},
	{
		src:  "a{1..4..-1}",
		want: []string{"a1", "a2", "a3", "a4"},
	},
	{
		src:  "a{4..1}",
		want: []string{"a4", "a3", "a2", "a1"},
	},
	{
		src:  "a{8..1..3}",
		want: []string{"a8", "a5", "a2"},
	},
	{
		src:  "a{4..1..-2}",
		want: []string{"a4", "a2"},
	},
	{
		src:  "a{4..1..1}",
		want: []string{"a4", "a3", "a2", "a1"},
	},
	{
		src:  "{1..005}",
		want: []string{"001", "002", "003", "004", "005"},
	},
	{
		src:  "{09..12}",
		want: []string{"09", "10", "11", "12"},
	},
	{
		src:  "{12..07}",
		want: []string{"12", "11", "10", "09", "08", "07"},
	},
	{
		src:  "{-02..4}",
		want: []string{"-02", "-01", "000", "001", "002", "003", "004"},
	},
	{
		src:  "{+02..4}",
		want: []string{"2", "3", "4"},
	},
	{
		src:  "{0001..05..2}",
		want: []string{"0001", "0003", "0005"},
	},
	{
		src:  "{0..1}",
		want: []string{"0", "1"},
	},
	{
		src:  "a{d..k..3}",
		want: []string{"ad", "ag", "aj"},
	},
	{
		src:  "a{a..e..-2}",
		want: []string{"aa", "ac", "ae"},
	},
	{
		src:  "a{d..k..n}",
		want: []string{"a{d..k..n}"},
	},
	{
		src:  "a{k..d..-2}",
		want: []string{"ak", "ai", "ag", "ae"},
	},
	{
		src:  "a{e..a..2}",
		want: []string{"ae", "ac", "aa"},
	},
	{
		src:  "{1..1}",
		want: []string{"1"},
	},
}

func TestBraces(t *testing.T) {
	t.Parallel()
	for _, tc := range braceTests {
		t.Run(tc.src, func(t *testing.T) {
			word := parseCommandWord(t, tc.src)
			inStr := printWords(word)
			wantStr := printWords(litWords(tc.want...)...)
			wantBraceExpParts(t, word, inStr != wantStr)

			got, err := Braces(word)
			if err != nil {
				t.Fatalf("Braces(%q) error = %v", tc.src, err)
			}
			gotStr := printWords(got...)
			if gotStr != wantStr {
				t.Fatalf("mismatch in %q\nwant:\n%s\ngot: %s",
					inStr, wantStr, gotStr)
			}
		})
	}
}

func TestBracesMixedCaseCharRangeErrors(t *testing.T) {
	t.Parallel()

	const wantMixedCaseErr = "bad substitution: no closing \"`\" in `-"

	tests := []struct {
		src  string
		want string
	}{
		{
			src:  `-{z..A}-`,
			want: wantMixedCaseErr,
		},
		{
			src:  `-{z..A..2}-`,
			want: wantMixedCaseErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			word := parseCommandWord(t, tc.src)
			got, err := Braces(word)
			if err == nil {
				t.Fatalf("Braces(%q) unexpectedly succeeded with %q", tc.src, printWords(got...))
			}
			if err.Error() != tc.want {
				t.Fatalf("Braces(%q) error = %q, want %q", tc.src, err.Error(), tc.want)
			}
		})
	}
}

func TestFieldsMixedCaseCharRangeError(t *testing.T) {
	t.Parallel()

	_, err := Fields(nil, parseCommandWord(t, `-{z..A}-`))
	if err == nil {
		t.Fatal("Fields() unexpectedly succeeded")
	}
	const want = "bad substitution: no closing \"`\" in `-"
	if err.Error() != want {
		t.Fatalf("Fields() error = %q, want %q", err.Error(), want)
	}
}

func TestBracesMixedCaseCharRangeWithoutSuffixExpands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src    string
		prefix string
		from   rune
		to     rune
		step   int
	}{
		{
			src:  `{z..A}`,
			from: 'z',
			to:   'A',
			step: 1,
		},
		{
			src:    `-{z..A}`,
			prefix: "-",
			from:   'z',
			to:     'A',
			step:   1,
		},
		{
			src:  `{z..A..2}`,
			from: 'z',
			to:   'A',
			step: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			word := parseCommandWord(t, tc.src)
			got, err := Braces(word)
			if err != nil {
				t.Fatalf("Braces(%q) error = %v", tc.src, err)
			}

			var want []string
			for r := tc.from; r >= tc.to; r -= rune(tc.step) {
				want = append(want, tc.prefix+string(r))
			}
			wantStr := printWords(litWords(want...)...)
			if gotStr := printWords(got...); gotStr != wantStr {
				t.Fatalf("Braces(%q) = %q, want %q", tc.src, gotStr, wantStr)
			}
		})
	}
}

func TestBracesInvalidSequenceStepLiteralizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src  string
		want []string
	}{
		{
			src:  "a{1..2..-9223372036854775808}b{c,d}",
			want: []string{"a{1..2..-9223372036854775808}bc", "a{1..2..-9223372036854775808}bd"},
		},
		{
			src:  "a{1..2..9223372036854775808}b{c,d}",
			want: []string{"a{1..2..9223372036854775808}bc", "a{1..2..9223372036854775808}bd"},
		},
	}

	for _, src := range tests {
		t.Run(src.src, func(t *testing.T) {
			word := parseCommandWord(t, src.src)
			got, err := Braces(word)
			if err != nil {
				t.Fatalf("Braces(%q) error = %v", src.src, err)
			}
			want := printWords(litWords(src.want...)...)
			if gotStr := printWords(got...); gotStr != want {
				t.Fatalf("Braces(%q) = %q, want %q", src.src, gotStr, want)
			}
		})
	}
}

func wantBraceExpParts(t *testing.T, word *syntax.Word, want bool) {
	t.Helper()
	anyBrace := false
	for _, part := range word.Parts {
		if _, anyBrace = part.(*syntax.BraceExp); anyBrace {
			break
		}
	}
	if anyBrace && !want {
		t.Fatalf("didn't want any BraceExp node, but found one")
	} else if !anyBrace && want {
		t.Fatalf("wanted a BraceExp node, but found none")
	}
}

func printWords(words ...*syntax.Word) string {
	p := syntax.NewPrinter()
	var buf bytes.Buffer
	call := &syntax.CallExpr{Args: words}
	p.Print(&buf, call)
	return buf.String()
}

func TestBracesSequenceNearIntBoundsTerminates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src  string
		want []string
	}{
		{
			src:  "{9223372036854775806..9223372036854775807}",
			want: []string{"9223372036854775806", "9223372036854775807"},
		},
		{
			src:  "{9223372036854775807..9223372036854775806}",
			want: []string{"9223372036854775807", "9223372036854775806"},
		},
		{
			src:  "{-9223372036854775807..-9223372036854775808}",
			want: []string{"-9223372036854775807", "-9223372036854775808"},
		},
		{
			src:  "{-9223372036854775808..-9223372036854775807}",
			want: []string{"-9223372036854775808", "-9223372036854775807"},
		},
		{
			src:  "{1..9223372036854775807..4611686018427387904}",
			want: []string{"1", "4611686018427387905"},
		},
		{
			src:  "{-1..-9223372036854775808..4611686018427387904}",
			want: []string{"-1", "-4611686018427387905"},
		},
		{
			src:  "{-9223372036854775808..9223372036854775807..9223372036854775807}",
			want: []string{"-9223372036854775808", "-1", "9223372036854775806"},
		},
		{
			src:  "x{9223372036854775807..9223372036854775807}y",
			want: []string{"x9223372036854775807y"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			// A small budget turns a regression into a fast failure instead of
			// an unbounded loop.
			got, err := braces(parseCommandWord(t, tc.src), newBraceBudget(16))
			if err != nil {
				t.Fatalf("braces(%q) error = %v", tc.src, err)
			}
			want := printWords(litWords(tc.want...)...)
			if gotStr := printWords(got...); gotStr != want {
				t.Fatalf("braces(%q) = %q, want %q", tc.src, gotStr, want)
			}
		})
	}
}

func TestFieldsBraceExpansionWordLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		words []string
		limit int64
		want  int
		fail  bool
	}{
		{name: "sequence at limit", words: []string{"{1..10}"}, limit: 10, want: 10},
		{name: "sequence over limit", words: []string{"{1..11}"}, limit: 10, fail: true},
		{name: "huge sequence", words: []string{"{1..9223372036854775807}"}, limit: 1000, fail: true},
		{name: "cartesian product", words: []string{"{1..4}{a,b,c}"}, limit: 10, fail: true},
		{name: "nested lists", words: []string{"{a,{b,{c,d}}}"}, limit: 4, want: 4},
		{name: "budget spans words", words: []string{"{1..6}", "{1..6}"}, limit: 10, fail: true},
		{name: "plain words not counted", words: []string{"a", "b", "c"}, limit: 1, want: 3},
		{name: "unlimited", words: []string{"{1..500}"}, limit: 0, want: 500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			words := make([]*syntax.Word, 0, len(tc.words))
			for _, src := range tc.words {
				words = append(words, parseCommandWord(t, src))
			}
			got, err := Fields(&Config{MaxBraceExpansionWords: tc.limit}, words...)
			if tc.fail {
				var limitErr BraceExpansionLimitError
				if !errors.As(err, &limitErr) {
					t.Fatalf("Fields() error = %v, want BraceExpansionLimitError", err)
				}
				if limitErr.Limit != tc.limit {
					t.Fatalf("Limit = %d, want %d", limitErr.Limit, tc.limit)
				}
				if want := fmt.Sprintf("brace expansion: too many words (limit %d)", tc.limit); err.Error() != want {
					t.Fatalf("Error() = %q, want %q", err.Error(), want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fields() error = %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("len(Fields()) = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func TestBraceExpansionScopeSpansFieldsCalls(t *testing.T) {
	t.Parallel()

	cfg := &Config{MaxBraceExpansionWords: 10}
	word := parseCommandWord(t, "{1..6}")

	// Outside a scope each call has its own budget.
	for range 3 {
		if _, err := Fields(cfg, word); err != nil {
			t.Fatalf("Fields() outside scope error = %v", err)
		}
	}

	cfg.BeginBraceExpansionScope()
	cfg.BeginBraceExpansionScope()
	if _, err := Fields(cfg, word); err != nil {
		t.Fatalf("first Fields() in scope error = %v", err)
	}
	cfg.EndBraceExpansionScope()
	var limitErr BraceExpansionLimitError
	if _, err := Fields(cfg, word); !errors.As(err, &limitErr) {
		t.Fatalf("second Fields() in scope error = %v, want BraceExpansionLimitError", err)
	}
	cfg.EndBraceExpansionScope()

	// A new outermost scope starts from zero again.
	cfg.BeginBraceExpansionScope()
	defer cfg.EndBraceExpansionScope()
	if _, err := Fields(cfg, word); err != nil {
		t.Fatalf("Fields() in fresh scope error = %v", err)
	}
}
