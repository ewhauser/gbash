package syntax

// heredocCache memoizes heredoc work that parsers repeat on the same text
// within one parse. It is shared by the parser that created it, its paren
// ambiguity probes and the heredoc body sub-parsers beneath them.
//
// To resolve "((" and "$((", parenAmbiguityStmt and parenAmbiguityWordPart
// parse the rest of the input three times: as arithmetic, as a subshell or
// command substitution, and finally as the chosen one. A heredoc body is
// parsed by a fresh sub-parser, which resolves every "((" in it the same way,
// and an unterminated heredoc body runs to the end of the input. So when each
// level of "$( ((" held a heredoc whose body held the next level, the parses
// multiplied by three with each level. Every pass over a heredoc body also
// parses each line that may hold its delimiter to describe a near-miss close,
// again resolving any "((" on it.
//
// Both results only depend on what the key holds, so a hit returns what a
// fresh parse would have:
//   - bodies holds the body parses of probes. Only probes use it, as their
//     trees are thrown away; the final parse builds its own tree, so no node
//     of it is shared. Like [rawPartCache], an entry serves any nesting and
//     pattern nesting depth that cannot reach the limits, as probes reach
//     the same body at many depths.
//   - closes holds newHeredocCloseCandidate results. A hit returns a copy.
type heredocCache struct {
	bodies map[hdocBodyKey][]*hdocBodyEntry
	closes map[hdocCloseKey]*HeredocCloseCandidate
}

type hdocBodyKey struct {
	src          string
	start        Pos
	lang         LangVariant
	keepComments bool
}

type hdocBodyEntry struct {
	word *Word
	err  error

	// The sub-parser's nesting state, so that a hit leaves its owner as a
	// fresh parse would.
	base          int
	nestPeak      int
	nestHigh      int
	patternBase   int
	patternHigh   int
	chainLinks    int
	nestingFailed bool
}

type hdocCloseKey struct {
	pos              Pos
	line, delim      string
	lang             LangVariant
	parseExtGlob     bool
	legacyBashCompat bool
}

// sharedHeredocCache returns the cache p shares with its probes and heredoc
// body sub-parsers, creating it if needed.
func (p *Parser) sharedHeredocCache() *heredocCache {
	if p.hdocCache == nil {
		p.hdocCache, p.ownsHdocCache = &heredocCache{}, true
	}
	return p.hdocCache
}

// lookupBody returns an entry that parsing the body key holds would
// reproduce for owner at its current nesting depths, or nil.
func (c *heredocCache) lookupBody(key hdocBodyKey, owner *Parser) *hdocBodyEntry {
	depth, patternDepth := owner.nestDepth, owner.patternDepth
	for _, e := range c.bodies[key] {
		if e.nestingFailed {
			// Keep the error at the position a fresh parse reports.
			if depth == e.base && patternDepth == e.patternBase {
				return e
			}
		} else if depth+e.nestHigh-e.base <= MaxNestingDepth &&
			patternDepth+e.patternHigh-e.patternBase <= MaxPatternNestingDepth {
			return e
		}
	}
	return nil
}

func (c *heredocCache) storeBody(key hdocBodyKey, e *hdocBodyEntry) {
	if c.bodies == nil {
		c.bodies = make(map[hdocBodyKey][]*hdocBodyEntry)
	}
	c.bodies[key] = append(c.bodies[key], e)
}

// replay applies the nesting state a cached body parse left behind to owner,
// shifted to the depth owner is at now.
func (e *hdocBodyEntry) replay(owner *Parser) {
	shift := func(v int) int {
		if v == 0 {
			return 0
		}
		return v - e.base + owner.nestDepth
	}
	owner.nestPeak = max(owner.nestPeak, shift(e.nestPeak))
	owner.nestHigh = max(owner.nestHigh, shift(e.nestHigh))
	owner.patternHigh = max(owner.patternHigh, e.patternHigh-e.patternBase+owner.patternDepth)
	owner.chainLinks += e.chainLinks
	if e.nestingFailed {
		owner.errPass(e.err)
		owner.nestingFailed = true
	}
}

// heredocCloseCandidate is newHeredocCloseCandidate for p's options, reusing
// an earlier result for the same line.
func (p *Parser) heredocCloseCandidate(rawPos Pos, rawLine, delim []byte) *HeredocCloseCandidate {
	c := p.sharedHeredocCache()
	key := hdocCloseKey{
		pos:              rawPos,
		line:             string(rawLine),
		delim:            string(delim),
		lang:             p.lang,
		parseExtGlob:     p.parseExtGlob,
		legacyBashCompat: p.legacyBashCompat,
	}
	candidate, ok := c.closes[key]
	if !ok {
		candidate = newHeredocCloseCandidate(rawPos, rawLine, delim, p.lang, p.parseExtGlob, p.legacyBashCompat)
		if c.closes == nil {
			c.closes = make(map[hdocCloseKey]*HeredocCloseCandidate)
		}
		c.closes[key] = candidate
	}
	if candidate == nil {
		return nil
	}
	cp := *candidate
	return &cp
}
