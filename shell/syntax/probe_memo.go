package syntax

// probeMemo memoizes the nested arithmetic that paren ambiguity probes parse
// without probing. It is shared by a parser and all of its probes, which
// read the same input.
//
// To resolve "((" and "$((", parenAmbiguityStmt and parenAmbiguityWordPart
// parse the rest of the input as arithmetic and as a subshell or command
// substitution, in clones that do not probe again below them. With each
// level of "$( ((" or "$((" holding the next one, every level's probes
// parsed all deeper levels again, so the parse was quadratic in the depth,
// with a large constant from the deep stacks. A probe that reaches a nested
// "$((" or "((" at the same offset in the same state parses it the same way,
// so later probes take the earlier result and move the lexer past it.
//
// Only probes use it, as their trees are thrown away: an entry's node is
// shared by every probe that hits it, and the final parse builds its own.
type probeMemo struct {
	entries map[probeMemoKey][]*probeMemoEntry
}

type probeMemoKind uint8

const (
	probeMemoArithmExp probeMemoKind = iota // "$((" word part
	probeMemoArithmCmd                      // "((" statement
)

// probeMemoKey holds the parser state that the parse of a nested construct
// depends on, besides the input from its offset on and the parser's options,
// which all users of one probeMemo share.
type probeMemoKey struct {
	offs              uint
	kind              probeMemoKind
	quote             quoteState
	forbidNested      bool
	deep              bool // parenAmbiguityProbeDepth > 1; see arithmExpCmd
	openBquotes       int
	openBquoteDquotes int
}

type probeMemoEntry struct {
	node Node
	err  error
	// end is where the lexer resumes after the construct, when err is nil.
	end Pos

	// The nesting state the construct left, relative to where it started,
	// so that a hit leaves its owner as a fresh parse would.
	base        int
	nestHigh    int
	patternBase int
	patternHigh int
	chainLinks  int
}

// probeMemoKey returns the key for parsing kind at the current token, and
// false when p is not a probe or its state is not one an entry may serve.
func (p *Parser) probeMemoKey(kind probeMemoKind) (probeMemoKey, bool) {
	if probeMemoOff || !p.parenAmbiguityDisabled || p.probeMemo == nil || p.sourceReplay == nil ||
		p.err != nil || p.rawSrc != nil || p.inputCut || p.keepComments || p.recoverErrorsMax > 0 ||
		len(p.heredocs) > 0 || p.buriedHdocs > 0 || len(p.hdocStops) > 0 ||
		p.aliasResolver != nil || len(p.aliasChain) > 0 || len(p.aliasInputStack) > 0 {
		return probeMemoKey{}, false
	}
	return probeMemoKey{
		offs:              p.pos.Offset(),
		kind:              kind,
		quote:             p.quote,
		forbidNested:      p.forbidNested,
		deep:              p.parenAmbiguityProbeDepth > 1,
		openBquotes:       p.openBquotes,
		openBquoteDquotes: p.openBquoteDquotes,
	}, true
}

// probeMemoOff turns the memo off, for tests that compare against it.
var probeMemoOff = false

// memoProbe runs parse, which parses the construct of kind at the current
// token, or replays an earlier probe's result for it.
func (p *Parser) memoProbe(kind probeMemoKind, parse func() Node) Node {
	key, ok := p.probeMemoKey(kind)
	if !ok {
		return parse()
	}
	for _, e := range p.probeMemo.entries[key] {
		if p.nestDepth+e.nestHigh-e.base > MaxNestingDepth ||
			p.patternDepth+e.patternHigh-e.patternBase > MaxPatternNestingDepth {
			continue
		}
		if p.replayProbeMemo(e) {
			return e.node
		}
	}

	base, patternBase := p.nestDepth, p.patternDepth
	oldPeak, oldHigh, oldPatternHigh, oldLinks := p.nestPeak, p.nestHigh, p.patternHigh, p.chainLinks
	p.nestPeak, p.nestHigh, p.patternHigh = base, base, patternBase
	node := parse()
	e := &probeMemoEntry{
		node:        node,
		err:         p.err,
		base:        base,
		nestHigh:    max(p.nestHigh, p.nestPeak),
		patternBase: patternBase,
		patternHigh: p.patternHigh,
		chainLinks:  p.chainLinks - oldLinks,
	}
	p.nestPeak = max(oldPeak, p.nestPeak)
	p.nestHigh = max(oldHigh, p.nestHigh)
	p.patternHigh = max(oldPatternHigh, p.patternHigh)

	if p.nestingFailed || isNilNode(node) || len(p.heredocs) > 0 || p.buriedHdocs > 0 || len(p.hdocStops) > 0 {
		return node
	}
	if e.err == nil {
		// Resuming at the construct's end re-lexes the token after it, so
		// only blanks may sit in between.
		e.end = node.End()
		from, to, ok := p.bufferRange(e.end, p.pos)
		if !ok || p.tok == _EOF {
			return node
		}
		buf, _ := p.sourceBuffer()
		for _, b := range buf[from:to] {
			if b != ' ' && b != '\t' {
				return node
			}
		}
	}
	if p.probeMemo.entries == nil {
		p.probeMemo.entries = make(map[probeMemoKey][]*probeMemoEntry)
	}
	p.probeMemo.entries[key] = append(p.probeMemo.entries[key], e)
	return node
}

// replayProbeMemo leaves p as parsing e's construct afresh would have, and
// reports false if it cannot.
func (p *Parser) replayProbeMemo(e *probeMemoEntry) bool {
	if e.err == nil {
		idx, ok := p.bufferIndex(e.end.Offset())
		if !ok {
			return false
		}
		buf, _ := p.sourceBuffer()
		replay := p.sourceReplay
		p.loadReplayCursorView(e.end, buf[idx:], replay.cursor(len(replay.Bytes())))
		p.next()
	}
	shift := e.nestHigh - e.base + p.nestDepth
	p.nestPeak = max(p.nestPeak, shift)
	p.nestHigh = max(p.nestHigh, shift)
	p.patternHigh = max(p.patternHigh, e.patternHigh-e.patternBase+p.patternDepth)
	p.chainLinks += e.chainLinks
	if e.err != nil {
		p.stopAtSubParserErr(e.err)
	}
	return true
}
