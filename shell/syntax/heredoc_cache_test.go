package syntax

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Every "((" and "$((" is parsed three times to resolve it as arithmetic or
// as a subshell, and heredoc bodies are parsed by fresh sub-parsers that
// resolve each "((" in them again. When every level of "$( ((" held an
// unterminated heredoc whose body held the next level, the parses multiplied
// with each level: the 175-byte mksh shape at 8 levels took 6.5s and the
// 304-byte zsh shape at 11 levels took 25s. These are fuzz finds from the
// unterminated-backticks attack, at about the most the fuzzers try.
func TestParenAmbiguityHeredocsParseInPolynomialTime(t *testing.T) {
	t.Parallel()
	rep := strings.Repeat
	tests := []struct {
		name string
		src  func(n int) string
		n    int
		opts []ParserOption
		// want is the error for n levels.
		want func(n int) string
	}{
		{
			name: "mksh subshell arithmetic",
			src: func(n int) string {
				return "`x\n$( ((( )<<EOF\n" + rep(" $( ($( ((( )<<EOF\n", n) + " $( (\n"
			},
			n:    200,
			opts: []ParserOption{Variant(LangMirBSDKorn), RecoverErrors(1)},
			want: func(n int) string { return fmt.Sprintf("%d:5: reached EOF without matching `(` with `)`", n+3) },
		},
		{
			name: "zsh paired heredocs",
			src: func(n int) string {
				return "`x\n$( (($( (<<EOF\n<<EOF\n" + rep("${x}$( (($( (<<EOF\n<<EOF\n", n) + "${x}\n"
			},
			n:    160,
			opts: []ParserOption{Variant(LangZsh), KeepComments(true), RecoverErrors(3)},
			want: func(n int) string { return fmt.Sprintf("%d:14: unclosed here-document `EOF`", 2*n+2) },
		},
		{
			name: "bash or-list subshells",
			src: func(n int) string {
				return "`x\n((<<EOF\n" + rep("${x}$($(echo x)|| (((<<EOF\n", n-1) + "${x}$($(echo x)|| (\n"
			},
			n:    150,
			opts: []ParserOption{Variant(LangBash), RecoverErrors(3)},
			want: func(n int) string { return fmt.Sprintf("%d:19: `(` must be followed by a statement list", n+2) },
		},
		{
			name: "mksh repeated heredocs",
			src: func(n int) string {
				return "`x\n" + rep("$( ((( )<<EOF\n<<EOF\n $( ", n) + "$(\n"
			},
			n:    160,
			opts: []ParserOption{Variant(LangMirBSDKorn), KeepComments(true), RecoverErrors(1)},
			want: func(n int) string { return fmt.Sprintf("%d:5: reached EOF without matching `$(` with `)`", 2*n+2) },
		},
		{
			name: "arithmetic expansion",
			src: func(n int) string {
				return rep("$(( $( (<<EOF\n", n) + "x\n"
			},
			n:    280,
			opts: []ParserOption{Variant(LangBash), RecoverErrors(3)},
			want: func(n int) string { return fmt.Sprintf("%d:9: unclosed here-document `EOF`", n) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The small depths pin the results the parser gave before the
			// heredoc cache, when they were still quick to compute.
			for _, n := range []int{3, 6, tc.n} {
				src := tc.src(n)
				done := make(chan error, 1)
				go func() {
					_, err := NewParser(tc.opts...).Parse(strings.NewReader(src), "")
					done <- err
				}()
				select {
				case err := <-done:
					if got, want := fmt.Sprint(err), tc.want(n); got != want {
						t.Errorf("%d levels (%d bytes): Parse error = %q, want %q", n, len(src), got, want)
					}
				case <-time.After(10 * time.Second):
					t.Fatalf("%d levels (%d bytes): Parse did not finish", n, len(src))
				}
			}
		})
	}
}

// A heredoc body that failed to parse set the error without stopping the
// lexer, so callExpr kept finding the same token it could not consume and
// looped forever. A FuzzParseAttackMutations find from process-subst-unclosed.
func TestHeredocBodyErrorStopsParse(t *testing.T) {
	t.Parallel()
	for _, n := range []int{1, 2, 8} {
		src := "cat <(echo hi\n" + strings.Repeat("${$(EOF\nEOF\n<<EOF\n ${$(EOF\n", n)
		for _, lang := range []LangVariant{LangZsh, LangBash} {
			done := make(chan error, 1)
			go func() {
				_, err := NewParser(Variant(lang)).Parse(strings.NewReader(src), "")
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Errorf("%v, %d levels: Parse error = nil, want one", lang, n)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%v, %d levels: Parse did not finish", lang, n)
			}
		}
	}
}

// After a heredoc body failed, parsing used to go on past the error, and a
// backquote recovery could then replace it with an error at a position that
// does not exist, such as column 5 of the three-byte line "EOF".
func TestHeredocBodyErrorKeepsFirstError(t *testing.T) {
	t.Parallel()
	body := "`$((1+2))${x} <<EOF\n $( { EOF\n"
	for _, src := range []string{
		"$(x\nEOF\n" + body,
		"$(x\nEOF\n" + body + "EOF\n" + body,
	} {
		_, err := NewParser(Variant(LangBash)).Parse(strings.NewReader(src), "")
		if got, want := fmt.Sprint(err), "4:5: reached EOF without matching `{` with `}`"; got != want {
			t.Errorf("Parse(%q) error = %q, want %q", src, got, want)
		}
	}
}
