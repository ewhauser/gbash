package syntax

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// walkRecursiveReference is the recursive Walk that predates the
// stack-based one, kept to check that the visiting order did not change.
func walkRecursiveReference(node Node, f func(Node) bool) {
	if node == nil || !f(node) {
		return
	}

	switch node := node.(type) {
	case *File:
		refWalkList(node.Stmts, f)
		refWalkComments(node.Last, f)
	case *Comment:
	case *Stmt:
		for _, c := range node.Comments {
			if !node.End().After(c.Pos()) {
				defer walkRecursiveReference(&c, f)
				break
			}
			walkRecursiveReference(&c, f)
		}
		if node.Cmd != nil {
			walkRecursiveReference(node.Cmd, f)
		}
		refWalkList(node.Redirs, f)
	case *VarRef:
		refWalkNilable(node.Name, f)
		refWalkNilable(node.Index, f)
	case *Subscript:
		refWalkNilable(node.Expr, f)
	case *Assign:
		refWalkNilable(node.Ref, f)
		refWalkNilable(node.Value, f)
		refWalkNilable(node.Array, f)
	case *DeclFlag:
		refWalkNilable(node.Word, f)
	case *DeclName:
		refWalkNilable(node.Ref, f)
	case *DeclAssign:
		refWalkNilable(node.Assign, f)
	case *DeclDynamicWord:
		refWalkNilable(node.Word, f)
	case *Redirect:
		refWalkNilable(node.N, f)
		refWalkNilable(node.Word, f)
		refWalkNilable(node.HdocDelim, f)
		refWalkNilable(node.Hdoc, f)
	case *HeredocDelim:
		refWalkList(node.Parts, f)
	case *CallExpr:
		refWalkList(node.Assigns, f)
		refWalkList(node.Args, f)
	case *Subshell:
		refWalkList(node.Stmts, f)
		refWalkComments(node.Last, f)
	case *Block:
		refWalkList(node.Stmts, f)
		refWalkComments(node.Last, f)
	case *IfClause:
		refWalkList(node.Cond, f)
		refWalkComments(node.CondLast, f)
		refWalkList(node.Then, f)
		refWalkComments(node.ThenLast, f)
		refWalkNilable(node.Else, f)
	case *WhileClause:
		refWalkList(node.Cond, f)
		refWalkComments(node.CondLast, f)
		refWalkList(node.Do, f)
		refWalkComments(node.DoLast, f)
	case *ForClause:
		refWalkNilable(node.Loop, f)
		refWalkList(node.Do, f)
		refWalkComments(node.DoLast, f)
	case *WordIter:
		walkRecursiveReference(node.Name, f)
		refWalkList(node.Items, f)
	case *CStyleLoop:
		refWalkNilable(node.Init, f)
		refWalkNilable(node.Cond, f)
		refWalkNilable(node.Post, f)
	case *BinaryCmd:
		refWalkNilable(node.X, f)
		refWalkNilable(node.Y, f)
	case *FuncDecl:
		refWalkNilable(node.Name, f)
		refWalkList(node.Names, f)
		refWalkNilable(node.Body, f)
	case *Word:
		refWalkList(node.Parts, f)
	case *Lit:
	case *SglQuoted:
	case *DblQuoted:
		refWalkList(node.Parts, f)
	case *BraceExp:
		refWalkList(node.Elems, f)
	case *Pattern:
		refWalkList(node.Parts, f)
	case *PatternAny:
	case *PatternSingle:
	case *PatternCharClass:
	case *PatternGroup:
		refWalkList(node.Patterns, f)
	case *CmdSubst:
		refWalkList(node.Stmts, f)
		refWalkComments(node.Last, f)
	case *ParamExp:
		refWalkNilable(node.Flags, f)
		refWalkNilable(node.Param, f)
		refWalkNilable(node.NestedParam, f)
		refWalkNilable(node.Index, f)
		if node.Slice != nil {
			refWalkNilable(node.Slice.Offset, f)
			refWalkNilable(node.Slice.Length, f)
		}
		if node.Repl != nil {
			refWalkNilable(node.Repl.Orig, f)
			refWalkNilable(node.Repl.With, f)
		}
		if node.Exp != nil {
			refWalkNilable(node.Exp.Word, f)
			refWalkNilable(node.Exp.Pattern, f)
		}
	case *ArithmExp:
		refWalkNilable(node.X, f)
	case *ArithmCmd:
		refWalkNilable(node.X, f)
	case *BinaryArithm:
		refWalkNilable(node.X, f)
		refWalkNilable(node.Y, f)
	case *CondBinary:
		refWalkNilable(node.X, f)
		refWalkNilable(node.Y, f)
	case *CondUnary:
		refWalkNilable(node.X, f)
	case *CondParen:
		walkRecursiveReference(node.X, f)
	case *CondWord:
		refWalkNilable(node.Word, f)
	case *CondVarRef:
		refWalkNilable(node.Ref, f)
	case *CondPattern:
		refWalkNilable(node.Pattern, f)
	case *CondRegex:
		refWalkNilable(node.Word, f)
	case *BinaryTest:
		refWalkNilable(node.X, f)
		refWalkNilable(node.Y, f)
	case *UnaryArithm:
		refWalkNilable(node.X, f)
	case *UnaryTest:
		refWalkNilable(node.X, f)
	case *ParenArithm:
		refWalkNilable(node.X, f)
	case *FlagsArithm:
		walkRecursiveReference(node.Flags, f)
		if node.X != nil {
			walkRecursiveReference(node.X, f)
		}
	case *ParenTest:
		refWalkNilable(node.X, f)
	case *CaseClause:
		walkRecursiveReference(node.Word, f)
		refWalkList(node.Items, f)
		refWalkComments(node.Last, f)
	case *CaseItem:
		for _, c := range node.Comments {
			if c.Pos().After(node.Pos()) {
				defer walkRecursiveReference(&c, f)
				break
			}
			walkRecursiveReference(&c, f)
		}
		refWalkList(node.Patterns, f)
		refWalkList(node.Stmts, f)
		refWalkComments(node.Last, f)
	case *TestClause:
		refWalkNilable(node.X, f)
	case *DeclClause:
		refWalkList(node.Operands, f)
	case *ArrayExpr:
		refWalkList(node.Elems, f)
		refWalkComments(node.Last, f)
	case *ArrayElem:
		for _, c := range node.Comments {
			if c.Pos().After(node.Pos()) {
				defer walkRecursiveReference(&c, f)
				break
			}
			walkRecursiveReference(&c, f)
		}
		refWalkNilable(node.Index, f)
		refWalkNilable(node.Value, f)
	case *ExtGlob:
		refWalkList(node.Patterns, f)
	case *ProcSubst:
		refWalkList(node.Stmts, f)
		refWalkComments(node.Last, f)
	case *TimeClause:
		refWalkNilable(node.Stmt, f)
	case *CoprocClause:
		refWalkNilable(node.Name, f)
		refWalkNilable(node.Stmt, f)
	case *LetClause:
		refWalkList(node.Exprs, f)
	case *TestDecl:
		refWalkNilable(node.Description, f)
		refWalkNilable(node.Body, f)
	default:
		panic(fmt.Sprintf("syntax.Walk: unexpected node type %T", node))
	}

	f(nil)
}

type refNilableNode interface {
	Node
	comparable // pointer nodes, which can be compared to nil
}

func refWalkNilable[N refNilableNode](node N, f func(Node) bool) {
	var zero N // nil
	if node != zero {
		walkRecursiveReference(node, f)
	}
}

func refWalkList[N Node](list []N, f func(Node) bool) {
	for _, node := range list {
		walkRecursiveReference(node, f)
	}
}

func refWalkComments(list []Comment, f func(Node) bool) {
	// Note that []Comment does not satisfy the generic constraint []Node.
	for i := range list {
		walkRecursiveReference(&list[i], f)
	}
}

func walkTrace(walk func(Node, func(Node) bool), root Node, skipLits bool) []string {
	var trace []string
	walk(root, func(n Node) bool {
		if n == nil {
			trace = append(trace, "nil")
			return true
		}
		trace = append(trace, fmt.Sprintf("%s@%d", reflect.TypeOf(n), n.Pos().Offset()))
		_, isLit := n.(*Lit)
		return !(skipLits && isLit)
	})
	return trace
}

func TestWalkMatchesRecursiveOrder(t *testing.T) {
	t.Parallel()
	var inputs []string
	for _, c := range append(append(fileTests, fileTestsKeepComments...), fileTestsNoPrint...) {
		inputs = append(inputs, c.inputs...)
	}
	for _, c := range printTests {
		inputs = append(inputs, c.in)
	}
	inputs = append(inputs,
		"# lead\nfoo # trail\n# after\nbar\n",
		"case x in\n# c1\na) b ;; # c2\n# c3\nesac\n",
		"a=(\n# c1\nx # c2\n# c3\ny\n)\n",
	)
	parser := NewParser(KeepComments(true))
	checked := 0
	for _, in := range inputs {
		prog, err := parser.Parse(strings.NewReader(in), "")
		if err != nil {
			continue
		}
		checked++
		for _, skipLits := range []bool{false, true} {
			want := walkTrace(walkRecursiveReference, prog, skipLits)
			got := walkTrace(Walk, prog, skipLits)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Walk order differs for %q (skipLits=%v):\ngot  %v\nwant %v", in, skipLits, got, want)
			}
		}
	}
	if checked < 100 {
		t.Fatalf("only %d inputs parsed, want a meaningful corpus", checked)
	}
}

// A left-deep chain is what the parser builds, in a loop, for "a && b && ...".
// The parser now caps its depth at MaxTreeDepth, but Walk is public and must
// not overflow the goroutine stack, a fatal error recover() cannot catch, on
// any tree it is handed.
func TestWalkDeepChainDoesNotOverflow(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds a large tree")
	}
	const n = 1_000_000
	call := func() *Stmt {
		return &Stmt{Cmd: &CallExpr{Args: []*Word{{Parts: []WordPart{&Lit{Value: "true"}}}}}}
	}
	s := call()
	for range n {
		s = &Stmt{Cmd: &BinaryCmd{Op: AndStmt, X: s, Y: call()}}
	}
	var binCmds, depth, maxDepth int
	Walk(&File{Stmts: []*Stmt{s}}, func(node Node) bool {
		if node == nil {
			depth--
			return true
		}
		depth++
		maxDepth = max(maxDepth, depth)
		if _, ok := node.(*BinaryCmd); ok {
			binCmds++
		}
		return true
	})
	if binCmds != n {
		t.Fatalf("visited %d BinaryCmd nodes, want %d", binCmds, n)
	}
	if depth != 0 || maxDepth < n {
		t.Fatalf("depth = %d, maxDepth = %d; want 0 and at least %d", depth, maxDepth, n)
	}
}
