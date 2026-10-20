package syntax

import (
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
		{name: "case bare groups", groups: true, ok: 300, opts: zsh, src: func(n int) string {
			return "case x in y" + rep("(a|", n) + "x" + rep(")", n) + ") ;; esac"
		}},
		// Shell parts are parsed once each, but every level still copies
		// and re-reads the text of the levels inside it, so these are
		// quadratic in their depth and only checked shallow.
		{name: "case in command substitution", ok: 30, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in $(X) ) ;; esac")
		}},
		{name: "case alternatives in command substitution", ok: 25, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in a|$(X)|b) ;; esac")
		}},
		{name: "quoted case in command substitution", ok: 25, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in \"$(X)\") ;; esac")
		}},
		{name: "case extglob in command substitution", ok: 25, src: func(n int) string {
			return nestCaseInCmdSubst(n, "case x in @($(X))) ;; esac")
		}},
		{name: "conditional in command substitution", ok: 40, src: func(n int) string {
			return nestCaseInCmdSubst(n, "[[ x == $(X) ]]")
		}},
		{name: "conditional extglob in command substitution", ok: 30, src: func(n int) string {
			return nestCaseInCmdSubst(n, "[[ x == @($(X)) ]]")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			depths := []int{tc.ok}
			if tc.groups {
				depths = append(depths, 2*MaxNestingDepth)
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
					if n > MaxNestingDepth {
						requireNestingError(t, err, MaxNestingDepth)
					}
				case <-time.After(10 * time.Second):
					t.Fatalf("depth %d: Parse did not finish", n)
				}
			}
		})
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
