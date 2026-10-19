// Copyright (c) 2016, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package syntax

import (
	"fmt"
	"io"
	"iter"
	"reflect"
	"slices"
	"sync"
)

// Walk traverses a syntax tree in depth-first order: It starts by calling
// f(node); node must not be nil. If f returns true, Walk invokes f
// recursively for each of the non-nil children of node, followed by
// f(nil).
//
// Walk keeps its own stack instead of recursing, so arbitrarily deep trees,
// such as long "a && b && ..." chains, cannot overflow the goroutine stack.
func Walk(node Node, f func(Node) bool) {
	if node == nil {
		return
	}
	w := walkerPool.Get().(*walker)
	w.stack = append(w.stack[:0], walkEntry{node: node})
	defer w.release()
	for len(w.stack) > 0 {
		e := w.stack[len(w.stack)-1]
		w.stack = w.stack[:len(w.stack)-1]
		if e.close {
			f(nil)
			continue
		}
		if e.node == nil || !f(e.node) {
			continue
		}
		w.push(e.node)
	}
}

// walkEntry is either a node still to visit or, when close is set, the
// f(nil) call that ends a visited node.
type walkEntry struct {
	node  Node
	close bool
}

type walker struct {
	stack []walkEntry
}

var walkerPool = sync.Pool{New: func() any { return new(walker) }}

func (w *walker) release() {
	if cap(w.stack) > 1<<12 {
		// Don't keep huge stacks from pathological trees alive.
		return
	}
	clear(w.stack[:cap(w.stack)])
	w.stack = w.stack[:0]
	walkerPool.Put(w)
}

// push schedules the children of node, then its closing f(nil), so that they
// pop off the stack in the order the recursive walk visited them. A trailing
// comment kept by a Stmt, CaseItem or ArrayElem is visited after the closing
// call, matching the deferred walk of the original recursive implementation.
func (w *walker) push(node Node) {
	// Comments attached to a Stmt, CaseItem or ArrayElem come first, except
	// that the first trailing one is visited after the closing f(nil), and
	// any later ones are not visited at all, as the recursive walk did.
	var comments []Comment
	var pos Pos
	atEnd := false
	switch node := node.(type) {
	case *Stmt:
		if comments = node.Comments; len(comments) > 0 {
			pos, atEnd = node.End(), true
		}
	case *CaseItem:
		comments, pos = node.Comments, node.Pos()
	case *ArrayElem:
		comments, pos = node.Comments, node.Pos()
	}
	leading := len(comments)
	for i := range comments {
		if isTrailingComment(&comments[i], pos, atEnd) {
			leading = i
			c := comments[i]
			w.add(&c)
			break
		}
	}
	w.stack = append(w.stack, walkEntry{close: true})
	base := len(w.stack)
	for i, c := range comments {
		if i == leading {
			break
		}
		w.add(&c)
	}
	w.appendChildren(node)
	slices.Reverse(w.stack[base:])
}

// isTrailingComment reports whether c follows its node: for a Stmt (atEnd),
// a comment at or after the statement's end; otherwise, one after pos.
func isTrailingComment(c *Comment, pos Pos, atEnd bool) bool {
	if atEnd {
		return !pos.After(c.Pos())
	}
	return c.Pos().After(pos)
}

func (w *walker) add(node Node) {
	w.stack = append(w.stack, walkEntry{node: node})
}

// appendChildren adds the children of node in visiting order. Comments
// attached to a Stmt, CaseItem or ArrayElem are handled by push.
func (w *walker) appendChildren(node Node) {
	switch node := node.(type) {
	case *File:
		addList(w, node.Stmts)
		w.comments(node.Last)
	case *Comment:
	case *Stmt:
		if node.Cmd != nil {
			w.add(node.Cmd)
		}
		addList(w, node.Redirs)
	case *VarRef:
		addNilable(w, node.Name)
		addNilable(w, node.Index)
	case *Subscript:
		addNilable(w, node.Expr)
	case *Assign:
		addNilable(w, node.Ref)
		addNilable(w, node.Value)
		addNilable(w, node.Array)
	case *DeclFlag:
		addNilable(w, node.Word)
	case *DeclName:
		addNilable(w, node.Ref)
	case *DeclAssign:
		addNilable(w, node.Assign)
	case *DeclDynamicWord:
		addNilable(w, node.Word)
	case *Redirect:
		addNilable(w, node.N)
		addNilable(w, node.Word)
		addNilable(w, node.HdocDelim)
		addNilable(w, node.Hdoc)
	case *HeredocDelim:
		addList(w, node.Parts)
	case *CallExpr:
		addList(w, node.Assigns)
		addList(w, node.Args)
	case *Subshell:
		addList(w, node.Stmts)
		w.comments(node.Last)
	case *Block:
		addList(w, node.Stmts)
		w.comments(node.Last)
	case *IfClause:
		addList(w, node.Cond)
		w.comments(node.CondLast)
		addList(w, node.Then)
		w.comments(node.ThenLast)
		addNilable(w, node.Else)
	case *WhileClause:
		addList(w, node.Cond)
		w.comments(node.CondLast)
		addList(w, node.Do)
		w.comments(node.DoLast)
	case *ForClause:
		addNilable(w, node.Loop)
		addList(w, node.Do)
		w.comments(node.DoLast)
	case *WordIter:
		w.add(node.Name)
		addList(w, node.Items)
	case *CStyleLoop:
		addNilable(w, node.Init)
		addNilable(w, node.Cond)
		addNilable(w, node.Post)
	case *BinaryCmd:
		addNilable(w, node.X)
		addNilable(w, node.Y)
	case *FuncDecl:
		addNilable(w, node.Name)
		addList(w, node.Names)
		addNilable(w, node.Body)
	case *Word:
		addList(w, node.Parts)
	case *Lit:
	case *SglQuoted:
	case *DblQuoted:
		addList(w, node.Parts)
	case *BraceExp:
		addList(w, node.Elems)
	case *Pattern:
		addList(w, node.Parts)
	case *PatternAny:
	case *PatternSingle:
	case *PatternCharClass:
	case *PatternGroup:
		addList(w, node.Patterns)
	case *CmdSubst:
		addList(w, node.Stmts)
		w.comments(node.Last)
	case *ParamExp:
		addNilable(w, node.Flags)
		addNilable(w, node.Param)
		addNilable(w, node.NestedParam)
		addNilable(w, node.Index)
		if node.Slice != nil {
			addNilable(w, node.Slice.Offset)
			addNilable(w, node.Slice.Length)
		}
		if node.Repl != nil {
			addNilable(w, node.Repl.Orig)
			addNilable(w, node.Repl.With)
		}
		if node.Exp != nil {
			addNilable(w, node.Exp.Word)
			addNilable(w, node.Exp.Pattern)
		}
	case *ArithmExp:
		addNilable(w, node.X)
	case *ArithmCmd:
		addNilable(w, node.X)
	case *BinaryArithm:
		addNilable(w, node.X)
		addNilable(w, node.Y)
	case *CondBinary:
		addNilable(w, node.X)
		addNilable(w, node.Y)
	case *CondUnary:
		addNilable(w, node.X)
	case *CondParen:
		w.add(node.X)
	case *CondWord:
		addNilable(w, node.Word)
	case *CondVarRef:
		addNilable(w, node.Ref)
	case *CondPattern:
		addNilable(w, node.Pattern)
	case *CondRegex:
		addNilable(w, node.Word)
	case *BinaryTest:
		addNilable(w, node.X)
		addNilable(w, node.Y)
	case *UnaryArithm:
		addNilable(w, node.X)
	case *UnaryTest:
		addNilable(w, node.X)
	case *ParenArithm:
		addNilable(w, node.X)
	case *FlagsArithm:
		w.add(node.Flags)
		if node.X != nil {
			w.add(node.X)
		}
	case *ParenTest:
		addNilable(w, node.X)
	case *CaseClause:
		w.add(node.Word)
		addList(w, node.Items)
		w.comments(node.Last)
	case *CaseItem:
		addList(w, node.Patterns)
		addList(w, node.Stmts)
		w.comments(node.Last)
	case *TestClause:
		addNilable(w, node.X)
	case *DeclClause:
		addList(w, node.Operands)
	case *ArrayExpr:
		addList(w, node.Elems)
		w.comments(node.Last)
	case *ArrayElem:
		addNilable(w, node.Index)
		addNilable(w, node.Value)
	case *ExtGlob:
		addList(w, node.Patterns)
	case *ProcSubst:
		addList(w, node.Stmts)
		w.comments(node.Last)
	case *TimeClause:
		addNilable(w, node.Stmt)
	case *CoprocClause:
		addNilable(w, node.Name)
		addNilable(w, node.Stmt)
	case *LetClause:
		addList(w, node.Exprs)
	case *TestDecl:
		addNilable(w, node.Description)
		addNilable(w, node.Body)
	default:
		panic(fmt.Sprintf("syntax.Walk: unexpected node type %T", node))
	}
}

type nilableNode interface {
	Node
	comparable // pointer nodes, which can be compared to nil
}

func addNilable[N nilableNode](w *walker, node N) {
	var zero N // nil
	if node != zero {
		w.add(node)
	}
}

func addList[N Node](w *walker, list []N) {
	for _, node := range list {
		w.add(node)
	}
}

func (w *walker) comments(list []Comment) {
	// Note that []Comment does not satisfy the generic constraint []Node.
	for i := range list {
		w.add(&list[i])
	}
}

// All returns an iterator over all nodes in the syntax tree rooted at node,
// in depth-first pre-order. Unlike [Walk], it does not yield nil sentinels
// and does not support skipping subtrees.
func All(node Node) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		var stopped bool
		Walk(node, func(n Node) bool {
			if stopped || n == nil {
				return !stopped
			}
			if !yield(n) {
				stopped = true
			}
			return !stopped
		})
	}
}

// DebugPrint prints the provided syntax tree, spanning multiple lines and with
// indentation. Can be useful to investigate the content of a syntax tree.
func DebugPrint(w io.Writer, node Node) error {
	p := debugPrinter{out: w}
	p.print(reflect.ValueOf(node))
	p.printf("\n")
	return p.err
}

type debugPrinter struct {
	out   io.Writer
	level int
	err   error
}

func (p *debugPrinter) printf(format string, args ...any) {
	_, err := fmt.Fprintf(p.out, format, args...)
	if err != nil && p.err == nil {
		p.err = err
	}
}

func (p *debugPrinter) newline() {
	p.printf("\n")
	for range p.level {
		p.printf(".  ")
	}
}

func (p *debugPrinter) print(x reflect.Value) {
	switch x.Kind() {
	case reflect.Interface:
		if x.IsNil() {
			p.printf("nil")
			return
		}
		p.print(x.Elem())
	case reflect.Pointer:
		if x.IsNil() {
			p.printf("nil")
			return
		}
		p.printf("*")
		p.print(x.Elem())
	case reflect.Slice:
		p.printf("%s (len = %d) {", x.Type(), x.Len())
		if x.Len() > 0 {
			p.level++
			p.newline()
			for i := range x.Len() {
				p.printf("%d: ", i)
				p.print(x.Index(i))
				if i == x.Len()-1 {
					p.level--
				}
				p.newline()
			}
		}
		p.printf("}")

	case reflect.Struct:
		if v, ok := x.Interface().(Pos); ok {
			if v.IsRecovered() {
				p.printf("<recovered>")
				return
			}
			p.printf("%v:%v", v.Line(), v.Col())
			return
		}
		t := x.Type()
		p.printf("%s {", t)
		exported := make([]int, 0, t.NumField())
		for i := range t.NumField() {
			if t.Field(i).IsExported() {
				exported = append(exported, i)
			}
		}
		if len(exported) == 0 {
			p.printf("}")
			return
		}
		p.level++
		p.newline()
		for i, fieldIndex := range exported {
			p.printf("%s: ", t.Field(fieldIndex).Name)
			p.print(x.Field(fieldIndex))
			if i == len(exported)-1 {
				p.level--
			}
			p.newline()
		}
		p.printf("}")
	default:
		if s, ok := x.Interface().(fmt.Stringer); ok && !x.IsZero() {
			p.printf("%#v (%s)", x.Interface(), s)
		} else {
			p.printf("%#v", x.Interface())
		}
	}
}
