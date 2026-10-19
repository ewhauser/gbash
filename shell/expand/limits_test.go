package expand

import (
	"errors"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/shell/syntax"
)

func TestReplaceParamPatternMaxVariableBytes(t *testing.T) {
	t.Parallel()

	for _, locale := range []string{"C.UTF-8", "C"} {
		t.Run(locale, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{
				Env:              ListEnviron("LC_ALL=" + locale),
				MaxVariableBytes: 16,
			}
			tests := []struct {
				pat, name, with string
				all             bool
				want            string
				wantErr         bool
			}{
				{pat: "?", name: "abcd", with: "xxxx", all: true, want: "xxxxxxxxxxxxxxxx"},
				{pat: "?", name: "abcde", with: "xxxx", all: true, wantErr: true},
				{pat: "a", name: "abab", with: "xxxxxxx", all: true, want: "xxxxxxxbxxxxxxxb"},
				{pat: "a", name: "ab", with: strings.Repeat("x", 16), wantErr: true},
				{pat: "z", name: strings.Repeat("a", 32), with: strings.Repeat("x", 32), all: true, want: strings.Repeat("a", 32)},
				{pat: "a", name: strings.Repeat("a", 32), with: "", all: true, want: ""},
				{pat: "a", name: strings.Repeat("a", 32), with: "b", all: true, want: strings.Repeat("b", 32)},
			}
			for _, tc := range tests {
				got, err := cfg.replaceParamPattern(tc.pat, tc.name, tc.with, tc.all, syntax.ReplaceAnchorNone)
				var limitErr VariableSizeLimitError
				if tc.wantErr {
					if !errors.As(err, &limitErr) || limitErr.Limit != 16 || limitErr.What != "pattern substitution" {
						t.Fatalf("replace(%q, %q, %q) error = %v, want limit error", tc.pat, tc.name, tc.with, err)
					}
					continue
				}
				if err != nil || got != tc.want {
					t.Fatalf("replace(%q, %q, %q) = %q, %v; want %q", tc.pat, tc.name, tc.with, got, err, tc.want)
				}
			}
		})
	}
}

func TestLimitedStringWriter(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	w := &limitedStringWriter{b: &sb, limit: 4}
	if _, err := w.Write([]byte("abcd")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := w.WriteString("e"); !errors.Is(err, ErrVariableSizeLimitWrite) {
		t.Fatalf("WriteString() error = %v, want ErrVariableSizeLimitWrite", err)
	}
	if !w.exceeded || sb.String() != "abcd" {
		t.Fatalf("exceeded = %v, buffer = %q", w.exceeded, sb.String())
	}
}
