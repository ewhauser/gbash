package syntax

import (
	"errors"
	"strings"
	"testing"
)

// Each input nests far deeper than the goroutine stack allows. They used to
// crash the process with "fatal error: stack overflow", which recover()
// cannot catch, either in the recursive descent parser itself or later in
// Walk or the interpreter; now they must fail with a ParseError.
func TestParseDeepNestingFailsWithParseError(t *testing.T) {
	t.Parallel()
	const n = 400_000
	rep := strings.Repeat
	tests := []struct {
		name string
		src  string
	}{
		{"arithmetic parens", "echo $((" + rep("(", n) + "1" + rep(")", n) + "))"},
		{"arithmetic unary minus", "echo $((" + rep("-", n) + "1))"},
		{"arithmetic unary minus spaced", "echo $((" + rep("- ", n) + "1))"},
		{"arithmetic not", "echo $((" + rep("!", n) + "1))"},
		{"arithmetic pre-increment", "echo $((" + rep("++ ", n) + "x))"},
		{"arithmetic assignment chain", "echo $((" + rep("a=", n) + "1))"},
		{"arithmetic power chain", "echo $((" + rep("2**", n) + "1))"},
		{"arithmetic ternary", "echo $((" + rep("1?", n) + "1" + rep(":1", n) + "))"},
		{"arithmetic ternary else", "echo $((" + rep("0?1:", n) + "1))"},
		{"subshell", rep("( ", n) + "echo x" + rep(" )", n)},
		{"block", rep("{ ", n) + "echo x;" + rep(" }", n)},
		{"if", rep("if true; then ", n) + "echo x" + rep("; fi", n)},
		{"command substitution", "echo " + rep("$(", n) + "echo x" + rep(")", n)},
		{"quoted command substitution", "echo " + rep(`"$(`, n) + "echo x" + rep(`)"`, n)},
		{"process substitution", "cat " + rep("<(", n) + "echo x" + rep(")", n)},
		{"parameter expansion", "echo " + rep("${x:-", n) + "y" + rep("}", n)},
		{"test negation", "[[ " + rep("! ", n) + "a ]]"},
		{"test parens", "[[ " + rep("( ", n) + "a" + rep(" )", n) + " ]]"},
		{"test and chain", "[[ a" + rep(" && a", n) + " ]]"},
		{"time", rep("time ", n) + "true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, opts := range [][]ParserOption{nil, {LegacyBashCompat(true), ParseExtGlob(true)}} {
				_, err := NewParser(opts...).Parse(strings.NewReader(tc.src+"\n"), "")
				requireNestingError(t, err, MaxNestingDepth)
			}
		})
	}
}

// Lists joined by operators are parsed in a loop, but still build one tree
// level per operator, and consumers such as the interpreter recurse on them.
func TestParseLongOperatorChainFailsWithParseError(t *testing.T) {
	t.Parallel()
	const n = 400_000
	tests := []struct {
		name string
		src  string
	}{
		{"and list", "true" + strings.Repeat(" && true", n)},
		{"or list", "true" + strings.Repeat(" || true", n)},
		{"pipeline", "echo a" + strings.Repeat(" | cat", n)},
		{"arithmetic sum", "echo $((1" + strings.Repeat("+1", n) + "))"},
		{"chain in command substitution", "echo $(true" + strings.Repeat(" && true", n) + ")"},
		{"chain in conditional pattern", "[[ x == $(true" + strings.Repeat(" && true", n) + ") ]]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewParser().Parse(strings.NewReader(tc.src+"\n"), "")
			requireNestingError(t, err, MaxTreeDepth)
		})
	}

	expr, err := NewParser().Arithmetic(strings.NewReader("1" + strings.Repeat("+1", n)))
	requireNestingError(t, err, MaxTreeDepth)
	_ = expr
}

func TestParseNestingWithinLimitsSucceeds(t *testing.T) {
	t.Parallel()
	const depth = 200
	rep := strings.Repeat
	for _, src := range []string{
		"echo $((" + rep("(", depth) + "1" + rep(")", depth) + "))",
		rep("( ", depth) + "echo x" + rep(" )", depth),
		"echo " + rep("${x:-", depth) + "y" + rep("}", depth),
		"[[ " + rep("! ", depth) + "a ]]",
		"true" + rep(" && true", MaxTreeDepth/4),
		"echo $((1" + rep("+1", MaxTreeDepth/4) + "))",
	} {
		if _, err := NewParser().Parse(strings.NewReader(src+"\n"), ""); err != nil {
			t.Errorf("Parse(%.40q...) error = %v, want nil", src, err)
		}
	}
}

func TestStmtsSeqChecksTreeDepthPerStatement(t *testing.T) {
	t.Parallel()
	src := "echo ok\n" + "true" + strings.Repeat(" && true", MaxTreeDepth+1) + "\n"
	var stmts int
	var gotErr error
	for stmt, err := range NewParser().StmtsSeq(strings.NewReader(src)) {
		if err != nil {
			gotErr = err
			break
		}
		if stmt != nil {
			stmts++
		}
	}
	requireNestingError(t, gotErr, MaxTreeDepth)
	if stmts != 1 {
		t.Fatalf("got %d statements before the error, want 1", stmts)
	}
}

// Patterns and heredoc bodies re-parse their shell parts with fresh
// sub-parsers, which must continue the nesting depth instead of starting
// over; otherwise nesting through them was unbounded. Deep input of this
// shape is slow to parse, so start the parser just below the limit.
func TestSubParsersInheritNestingDepth(t *testing.T) {
	t.Parallel()
	const levels = 4
	rep := strings.Repeat
	for _, src := range []string{
		"[[ x == " + rep("$([[ x == ", levels) + "x" + rep(" ]])", levels) + " ]]",
		"[[ x == " + rep(`"$([[ x == `, levels) + "x" + rep(` ]])"`, levels) + " ]]",
		"case x in " + rep("$(case x in ", levels) + "x" + rep(") ;; esac)", levels) + ") ;; esac",
		"cat <<EOF\n" + rep("$(", levels) + "x" + rep(")", levels) + "\nEOF",
	} {
		p := NewParser()
		p.nestBase = MaxNestingDepth - levels
		_, err := p.Parse(strings.NewReader(src+"\n"), "")
		requireNestingError(t, err, MaxNestingDepth)

		if _, err := NewParser().Parse(strings.NewReader(src+"\n"), ""); err != nil {
			t.Fatalf("Parse(%.40q...) from depth 0 error = %v, want nil", src, err)
		}
	}
}

func requireNestingError(t *testing.T, err error, limit int) {
	t.Helper()
	var parseErr ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v (%T), want a ParseError", err, err)
	}
	if want := nestingErrText(limit); !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to mention %q", err, want)
	}
}
