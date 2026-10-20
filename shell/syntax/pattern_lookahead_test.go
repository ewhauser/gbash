package syntax

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// Case patterns were only scanned within the parser's 1KiB read buffer, and a
// pattern running past it fell back to a parse that failed on a shell part cut
// off by the buffer's end, so bash scripts like these did not parse:
//
//	case x in $(<1KiB of commands>) ) ;; esac
func TestCasePatternsPastReadBuffer(t *testing.T) {
	t.Parallel()
	cmds := strings.Repeat("echo aaaa;", 110)
	tests := []struct {
		name string
		src  string
		opts []ParserOption
	}{
		{name: "command substitution", src: "case x in $(" + cmds + "case x in x) ;; esac) ) ;; esac"},
		{name: "alternatives", src: "case x in a|$(" + cmds + ")|b) y ;; c) ;; esac"},
		{name: "optional opener", src: "case x in ($(" + cmds + ")) ;; esac"},
		{name: "extglob", src: "case x in @($(" + cmds + "))) ;; esac"},
		{name: "quoted", src: "case x in \"$(" + cmds + ")\") ;; esac"},
		{name: "backquotes", src: "case x in `" + cmds + "`) ;; esac"},
		{name: "process substitution", src: "case x in <(" + cmds + ")) ;; esac"},
		{name: "arithmetic", src: "case x in $((" + strings.Repeat("1+", 600) + "1))) ;; esac"},
		{name: "char class", src: "case x in [" + strings.Repeat("a", 1100) + "]) ;; esac"},
		{name: "long first word", src: "case x in " + strings.Repeat("a", 3000) + "@($(" + cmds + "))) ;; esac"},
		{name: "zsh bare groups", opts: []ParserOption{Variant(LangZsh)}, src: "case x in y" + strings.Repeat("(a|", 300) + "x" + strings.Repeat(")", 300) + ") ;; esac"},
		{name: "nested command substitutions", src: nestCaseInCmdSubst(60, "case x in $(X) ) ;; esac")},
		{name: "nested alternatives", src: nestCaseInCmdSubst(60, "case x in a|$(X)|b) ;; esac")},
		{name: "nested extglobs", src: nestCaseInCmdSubst(60, "case x in @($(X))) ;; esac")},
		{name: "nested opener", src: nestCaseInCmdSubst(60, "case x in ($(X)) ;; esac")},
	}
	readers := []struct {
		name string
		wrap func(io.Reader) io.Reader
	}{
		{name: "whole", wrap: func(r io.Reader) io.Reader { return r }},
		// Short reads leave less in the read buffer than it can hold.
		{name: "half", wrap: iotest.HalfReader},
		{name: "one byte", wrap: iotest.OneByteReader},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := countPatternNodes(t, "", tc.opts, strings.NewReader(tc.src))
			// Padding moves the end of the read buffer to other places
			// in the pattern.
			for pad := 0; pad < bufSize; pad += 61 {
				for _, rd := range readers {
					src := strings.Repeat(" ", pad) + tc.src + "\n"
					desc := fmt.Sprintf("pad %d, %s reader", pad, rd.name)
					if got := countPatternNodes(t, desc, tc.opts, rd.wrap(strings.NewReader(src))); got != want {
						t.Fatalf("pad %d, %s reader: got %v, want %v", pad, rd.name, got, want)
					}
				}
			}
		})
	}
}

// Deeply nested case statements in command substitutions in case patterns
// parse, however far they run past the read buffer.
func TestDeepCasePatternsPastReadBuffer(t *testing.T) {
	t.Parallel()
	for _, item := range []string{
		"case x in $(X) ) ;; esac",
		"case x in @($(X))) ;; esac",
	} {
		const n = 1000
		src := nestCaseInCmdSubst(n, item) + "\n"
		f, err := NewParser().Parse(strings.NewReader(src), "")
		if err != nil {
			t.Fatalf("%d levels of %q: Parse error = %v", n, item, err)
		}
		clauses := 0
		Walk(f, func(node Node) bool {
			if _, ok := node.(*CaseClause); ok {
				clauses++
			}
			return true
		})
		if clauses != n {
			t.Fatalf("%d levels of %q: got %d case clauses", n, item, clauses)
		}
	}
}

type patternNodeCounts struct {
	caseClauses, caseItems, patterns, cmdSubsts, extGlobs, groups int
}

func countPatternNodes(t *testing.T, desc string, opts []ParserOption, r io.Reader) patternNodeCounts {
	t.Helper()
	f, err := NewParser(opts...).Parse(r, "")
	if err != nil {
		t.Fatalf("%s: Parse error = %v", desc, err)
	}
	var c patternNodeCounts
	Walk(f, func(node Node) bool {
		switch node := node.(type) {
		case *CaseClause:
			c.caseClauses++
		case *CaseItem:
			c.caseItems++
			c.patterns += len(node.Patterns)
		case *CmdSubst:
			c.cmdSubsts++
		case *ExtGlob:
			c.extGlobs++
		case *PatternGroup:
			c.groups++
		}
		return true
	})
	if c.caseClauses == 0 || c.patterns == 0 {
		t.Fatalf("%s: Parse found no case patterns", desc)
	}
	return c
}

// Reading ahead for a pattern must hand the bytes back to the input
// unchanged, including in what follows the case statement.
func TestCasePatternPastReadBufferKeepsSource(t *testing.T) {
	t.Parallel()
	cmds := strings.Repeat("echo aaaa;", 200)
	src := "case x in $(" + cmds + ") ) echo in ;; esac\necho after \"$(" + cmds + ")\"\n"
	for _, wrap := range []func(io.Reader) io.Reader{
		func(r io.Reader) io.Reader { return r },
		iotest.HalfReader,
		iotest.OneByteReader,
	} {
		f, err := NewParser().Parse(wrap(strings.NewReader(src)), "")
		if err != nil {
			t.Fatalf("Parse error = %v", err)
		}
		var sb strings.Builder
		if err := NewPrinter().Print(&sb, f); err != nil {
			t.Fatal(err)
		}
		f2, err := NewParser().Parse(strings.NewReader(sb.String()), "")
		if err != nil {
			t.Fatalf("Parse(printed) error = %v", err)
		}
		if len(f.Stmts) != 2 || len(f2.Stmts) != 2 {
			t.Fatalf("got %d and %d statements, want 2", len(f.Stmts), len(f2.Stmts))
		}
		last := f.Stmts[1].Cmd.(*CallExpr)
		if got := last.Args[1].Lit(); got != "after" {
			t.Fatalf("second statement argument = %q, want after", got)
		}
		if end := f.Stmts[1].End().Offset(); int(end) != len(src)-1 {
			t.Fatalf("second statement ends at %d, want %d", end, len(src)-1)
		}
	}
}

// A read error met while reading ahead for a pattern is still reported, once
// the parser gets there.
func TestCasePatternReadAheadError(t *testing.T) {
	t.Parallel()
	src := "case x in $(" + strings.Repeat("echo aaaa;", 200) + ") ) ;; esac\necho "
	r := io.MultiReader(strings.NewReader(src), iotest.ErrReader(iotest.ErrTimeout))
	_, err := NewParser().Parse(r, "")
	if err != iotest.ErrTimeout {
		t.Fatalf("Parse error = %v, want %v", err, iotest.ErrTimeout)
	}
}
