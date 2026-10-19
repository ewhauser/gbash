package syntax

import (
	"fmt"
	"reflect"
)

// MaxNestingDepth bounds how deeply the parser recurses into nested
// constructs such as subshells, blocks, command substitutions, parameter
// expansions, arithmetic parentheses and unary operators, and [[ ]]
// negations. Deeper input fails with a [ParseError] instead of overflowing
// the goroutine stack, which Go cannot recover from.
const MaxNestingDepth = 1000

// MaxTreeDepth bounds the depth of a parsed syntax tree. Lists such as
// "a && b && ..." or "1+1+..." are parsed in a loop yet still build one tree
// level per operator, and the interpreter walks those levels recursively, so
// they are capped separately from [MaxNestingDepth].
const MaxTreeDepth = 10000

// treeCheckThreshold is how much nesting or how many chained operators a
// top-level parse may see before its tree depth is measured. Below it the
// tree is shallow by construction, so ordinary scripts skip the extra walk.
const treeCheckThreshold = 256

// enterNest records one more level of nested parsing. It returns the depth
// to restore with leaveNest, and false once the input nests deeper than
// MaxNestingDepth, after recording a parse error that stops parsing.
func (p *Parser) enterNest() (int, bool) {
	d := p.nestDepth
	if d >= MaxNestingDepth {
		p.nestingErr(p.pos, MaxNestingDepth)
		return d, false
	}
	p.nestDepth = d + 1
	p.nestPeak = max(p.nestPeak, p.nestDepth)
	return d, true
}

// leaveNest restores the depth enterNest returned. Restoring an absolute
// value keeps the count right across parser snapshots and clones.
func (p *Parser) leaveNest(d int) {
	p.nestDepth = d
}

func (p *Parser) nestingErr(pos Pos, limit int) {
	p.posErr(pos, "%s", nestingErrText(limit))
	p.nestingFailed = true
}

func nestingErrText(limit int) string {
	return fmt.Sprintf("nesting exceeds the maximum depth of %d", limit)
}

// subParserNesting carries the nesting state into a fresh parser that
// continues parsing part of the same input, such as a heredoc body.
func (p *Parser) subParserNesting(sub *Parser) {
	sub.nestBase = p.nestDepth
}

// absorbSubParserNesting records what a sub-parser saw so that this parser's
// tree-depth check still runs when the sub-parser's output is deep, and so
// that a nesting error is not lost when the caller falls back to treating
// the sub-parser's input as literal text.
func (p *Parser) absorbSubParserNesting(sub *Parser) {
	p.nestPeak = max(p.nestPeak, sub.nestPeak)
	p.chainLinks += sub.chainLinks
	if sub.nestingFailed {
		p.errPass(sub.err)
		p.nestingFailed = true
	}
}

// checkTreeDepth fails the parse when node, a finished top-level result, is
// deeper than MaxTreeDepth. It only walks the tree when this parse saw
// enough nesting or chained operators for that to be possible.
func (p *Parser) checkTreeDepth(node Node) {
	seen := p.nestPeak + p.chainLinks
	p.nestPeak, p.chainLinks = 0, 0
	if p.err != nil || seen < treeCheckThreshold || isNilNode(node) {
		return
	}
	if deep := deepestNode(node, MaxTreeDepth); deep != nil {
		p.nestingErr(deep.Pos(), MaxTreeDepth)
	}
}

// deepestNode returns the first node found more than limit levels below
// root, or nil if the tree is no deeper than that.
func deepestNode(root Node, limit int) Node {
	var deep Node
	depth := 0
	Walk(root, func(n Node) bool {
		if n == nil {
			depth--
			return true
		}
		if deep != nil {
			return false
		}
		if depth++; depth > limit {
			deep = n
			return false
		}
		return true
	})
	return deep
}

func isNilNode(node Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
