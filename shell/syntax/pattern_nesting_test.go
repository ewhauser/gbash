package syntax

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func nestCaseInCmdSubst(n int, item string) string {
	s := "x"
	for range n {
		s = strings.ReplaceAll(item, "X", s)
	}
	return s
}

// Pattern parsing scans a pattern several times and used to re-parse every
// nested group and shell part on each scan, so these inputs took time
// exponential in their depth: 22 levels of @( took about 2s and 8 levels of
// case in $( about 1s.
func TestNestedPatternsParseInLinearTime(t *testing.T) {
	t.Parallel()
	rep := strings.Repeat
	extGlob := func(n int) string { return rep("@(", n) + "x" + rep(")", n) }
	zsh := []ParserOption{Variant(LangZsh)}
	tests := []struct {
		name string
		src  func(n int) string
		opts []ParserOption
		// ok is a depth that must parse without error. Groups are also
		// parsed at twice MaxNestingDepth, where they must fail.
		ok     int
		groups bool
		// otherErr allows the deep input to fail for another reason.
		otherErr bool
		// shell nests patterns through shell parts. These are also parsed
		// past MaxPatternNestingDepth, where they must fail on it.
		shell bool
	}{
		{name: "conditional extglob", groups: true, ok: 500, src: func(n int) string { return "[[ x == " + extGlob(n) + " ]]" }},
		{name: "case extglob", groups: true, ok: 500, src: func(n int) string { return "case x in " + extGlob(n) + ") ;; esac" }},
		{name: "word extglob", groups: true, ok: 500, src: func(n int) string { return "echo " + extGlob(n) }},
		{name: "extglob arms", groups: true, ok: 300, src: func(n int) string {
			return "[[ x == " + rep("@(a|", n) + "x" + rep("|b)", n) + " ]]"
		}},
		{name: "bare groups", groups: true, ok: 300, opts: zsh, src: func(n int) string {
			return "[[ x == " + rep("(a|", n) + "x" + rep(")", n) + " ]]"
		}},
		// Past 1KiB this case pattern crosses the parser's 1KiB read
		// buffer, which it cannot parse yet.
		{name: "case bare groups", groups: true, otherErr: true, ok: 200, opts: zsh, src: func(n int) string {
			return "case x in y" + rep("(a|", n) + "x" + rep(")", n) + ") ;; esac"
		}},
		// Shell parts are parsed once each, but every level re-parses the
		// text of the levels inside it, so their depth has its own, lower
		// limit; see TestPatternShellNestingIsCapped. Case statements stay
		// within 1KiB: a case pattern that crosses the parser's 1KiB read
		// buffer can fail to parse.
		{name: "case in command substitution", shell: true, ok: 30, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in $(X) ) ;; esac")
		}},
		{name: "case alternatives in command substitution", shell: true, ok: 25, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in a|$(X)|b) ;; esac")
		}},
		{name: "quoted case in command substitution", shell: true, ok: 25, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in \"$(X)\") ;; esac")
		}},
		{name: "case extglob in command substitution", shell: true, ok: 25, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in @($(X))) ;; esac")
		}},
		{name: "conditional in command substitution", shell: true, ok: MaxPatternNestingDepth, src: func(n int) string {
			return nestCaseInCmdSubst(n, "[[ x == $(X) ]]")
		}},
		{name: "conditional extglob in command substitution", shell: true, ok: MaxPatternNestingDepth, src: func(n int) string {
			return nestCaseInCmdSubst(n, "[[ x == @($(X)) ]]")
		}},
		{name: "word extglob in command substitution", shell: true, ok: MaxPatternNestingDepth, src: func(n int) string {
			return nestCaseInCmdSubst(n, "echo @($(X))")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			depths := []int{tc.ok}
			if tc.groups {
				depths = append(depths, 2*MaxNestingDepth)
			}
			if tc.shell {
				depths = append(depths, 4*MaxPatternNestingDepth)
			}
			for _, n := range depths {
				src := tc.src(n) + "\n"
				done := make(chan error, 1)
				go func() {
					_, err := NewParser(tc.opts...).Parse(strings.NewReader(src), "")
					done <- err
				}()
				select {
				case err := <-done:
					if n == tc.ok && err != nil {
						t.Errorf("depth %d: Parse error = %v, want nil", n, err)
					}
					if n > MaxNestingDepth && !tc.otherErr {
						requireNestingError(t, err, MaxNestingDepth)
					}
					if tc.shell && n > MaxPatternNestingDepth {
						requirePatternNestingError(t, err)
					}
				case <-time.After(10 * time.Second):
					t.Fatalf("depth %d: Parse did not finish", n)
				}
			}
		})
	}
}

// Each shell part in a pattern is re-parsed by a sub-parser that reads the
// text of every level inside it, so patterns nested through shell parts took
// time quadratic in their depth: minutes for case statements at
// MaxNestingDepth. Their depth is capped at MaxPatternNestingDepth.
func TestPatternShellNestingIsCapped(t *testing.T) {
	t.Parallel()
	shapes := []string{
		"[[ x == $(X) ]]",
		"[[ x == @($(X)) ]]",
		"[[ x == \"$(X)\" ]]",
		"[[ x == ${y:-$(X)} ]]",
		"[[ x == `X` ]]",
		"echo @($(X))",
		"echo @(<(X))",
		"case x in $(X) ) ;; esac",
		"case x in a|$(X)|b) ;; esac",
		"case x in \"$(X)\") ;; esac",
		"case x in @($(X))) ;; esac",
		// Levels that alternate between shapes share no cache entries.
		"case x in $([[ x == $(X) ]]) ) ;; esac",
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			for _, n := range []int{MaxPatternNestingDepth + 1, 4 * MaxPatternNestingDepth} {
				src := nestCaseInCmdSubst(n, shape) + "\n"
				done := make(chan error, 1)
				go func() {
					_, err := NewParser().Parse(strings.NewReader(src), "")
					done <- err
				}()
				select {
				case err := <-done:
					requirePatternNestingError(t, err)
				case <-time.After(30 * time.Second):
					t.Fatalf("depth %d: Parse did not finish", n)
				}
			}
		})
	}
}

// Past the cap, the levels outside it still read the text of every level
// inside, so input at MaxNestingDepth costs MaxPatternNestingDepth times its
// length instead of its depth times its length. Not parallel, so that other
// tests do not skew the time.
func TestPatternShellNestingFailsFastAtNestingLimit(t *testing.T) {
	for _, shape := range []string{
		// Took minutes before the cap.
		"case x in $(X) ) ;; esac",
		// Each level is two nesting levels, so this reaches
		// MaxNestingDepth at half the depth.
		"[[ x == $(X) ]]",
	} {
		src := nestCaseInCmdSubst(MaxNestingDepth, shape) + "\n"
		start := time.Now()
		_, err := NewParser().Parse(strings.NewReader(src), "")
		requirePatternNestingError(t, err)
		// Well under a second without the race detector; generous for
		// slow CI runners.
		if d := time.Since(start); d > 30*time.Second {
			t.Errorf("%q: Parse took %v", shape, d)
		}
	}
}

// Shapes that the parser accepts at MaxPatternNestingDepth fail one level
// deeper with the same error a fresh parse at that level reports, wherever
// the cached shell parts were first parsed.
func TestPatternShellNestingLimitIsExact(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{
		"[[ x == $(X) ]]",
		"[[ x == @($(y)|$(X)) ]]",
		"echo @($(X))",
	} {
		for _, n := range []int{1, MaxPatternNestingDepth} {
			src := nestCaseInCmdSubst(n, strings.Replace(shape, "X", "x", 1)) + "\n"
			if _, err := NewParser().Parse(strings.NewReader(src), ""); err != nil {
				t.Errorf("%q at depth %d: Parse error = %v, want nil", shape, n, err)
			}
		}
		src := nestCaseInCmdSubst(MaxPatternNestingDepth+1, shape) + "\n"
		_, err := NewParser().Parse(strings.NewReader(src), "")
		requirePatternNestingError(t, err)
	}
	// A heredoc body continues the depth of the parser that holds it. Its
	// outermost [[ is plain text, so the body nests one level less.
	inner := nestCaseInCmdSubst(MaxPatternNestingDepth+1, "[[ x == $(X) ]]")
	src := "[[ x == $(cat <<EOF\n" + inner + "\nEOF\n) ]]\n"
	_, err := NewParser().Parse(strings.NewReader(src), "")
	requirePatternNestingError(t, err)
}

// A cached shell part is reused only by owners at pattern depths where a
// fresh parse gives the same result.
func TestRawPartCacheRespectsPatternDepth(t *testing.T) {
	t.Parallel()
	// Parsing raw starts a sub-parser one level down, and its pattern
	// another, so an owner at MaxPatternNestingDepth-1 hits the limit.
	const raw = "$([[ x == $(y) ]])"
	parse := func(cache *rawPartCache, depth int) error {
		owner := NewParser()
		owner.rawParts = cache
		owner.patternDepth = depth
		e := rawWordPart(raw, Pos{}, LangBash, owner)
		if e.ok != (owner.err == nil) {
			t.Fatalf("depth %d: ok = %v with error %v", depth, e.ok, owner.err)
		}
		return owner.err
	}
	for _, order := range [][]int{
		{MaxPatternNestingDepth - 2, MaxPatternNestingDepth - 1},
		{MaxPatternNestingDepth - 1, MaxPatternNestingDepth - 2},
	} {
		cache := newRawPartCache()
		for _, depth := range order {
			err := parse(cache, depth)
			if depth == MaxPatternNestingDepth-1 {
				requirePatternNestingError(t, err)
			} else if err != nil {
				t.Errorf("depth %d after %v: error = %v, want nil", depth, order, err)
			}
		}
	}
}

func requirePatternNestingError(t *testing.T, err error) {
	t.Helper()
	var parseErr ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v (%T), want a ParseError", err, err)
	}
	if !strings.Contains(err.Error(), patternNestingErrText) {
		t.Fatalf("error = %q, want it to mention %q", err, patternNestingErrText)
	}
}

// Groups inside patterns are parsed recursively, so they count toward
// MaxNestingDepth like other nested constructs.
func TestParseDeepPatternNestingFailsWithParseError(t *testing.T) {
	t.Parallel()
	const n = 400_000
	rep := strings.Repeat
	extGlob := rep("@(", n) + "x" + rep(")", n)
	for _, src := range []string{
		"[[ x == " + extGlob + " ]]",
		"case x in " + extGlob + ") ;; esac",
		"echo " + extGlob,
		"[[ x == " + rep("@(a|", n) + "x" + rep(")", n) + " ]]",
	} {
		_, err := NewParser().Parse(strings.NewReader(src+"\n"), "")
		requireNestingError(t, err, MaxNestingDepth)
	}
}

// Shell parts in patterns are parsed once and shared between the scans of a
// pattern; the tree that is kept must still hold each node only once.
func TestPatternShellPartsAreNotShared(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		nestCaseInCmdSubst(5, "case x in a|$(X)|b) ;; esac"),
		nestCaseInCmdSubst(5, "case x in @($(X)|\"$y\")) ;; esac"),
		nestCaseInCmdSubst(5, "[[ x == @($(X)|${y}|'z') ]]"),
		"case x in $(a)|$(a) ) ;; ${b} | ${b} ) ;; esac",
		"echo @($(a)|$((1+2))|`b`)",
	} {
		f, err := NewParser().Parse(strings.NewReader(src+"\n"), "")
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", src, err)
		}
		seen := make(map[Node]bool)
		Walk(f, func(node Node) bool {
			if node == nil {
				return true
			}
			if seen[node] {
				t.Fatalf("Parse(%q): %T at %s appears twice in the tree", src, node, node.Pos())
			}
			seen[node] = true
			return true
		})
	}
}
