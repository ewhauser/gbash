// Copyright (c) 2026, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package pattern

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Match reports whether name matches pat under the supplied shell pattern mode.
// It falls back to the recursive matcher for patterns or inputs containing
// invalid UTF-8 bytes, since Go's regexp engine operates on UTF-8 code points.
func Match(pat, name string, mode Mode) (bool, error) {
	if utf8.ValidString(pat) && utf8.ValidString(name) || mode&Filenames != 0 {
		matcher, err := ExtendedPatternMatcher(pat, mode)
		if err != nil {
			return false, err
		}
		return matcher(name), nil
	}
	root, err := parseExtPattern(pat, mode)
	if err != nil {
		return false, err
	}
	return newExtMatcher(mode, name).matches(root), nil
}

// ExtendedPatternMatcher returns a [regexp.Regexp.MatchString]-like function.
// It falls back to a recursive matcher when [Regexp] reports a negated extglob
// !(...) group, since Go's regexp package cannot express that directly.
func ExtendedPatternMatcher(pat string, mode Mode) (func(string) bool, error) {
	if mode&ExtendedOperators != 0 && mode&EntireString == 0 {
		panic("ExtendedOperators is only supported with EntireString")
	}

	expr, err := Regexp(pat, mode)
	if err == nil {
		rx, err := regexp.Compile(expr)
		if err != nil {
			return nil, &SyntaxError{msg: "invalid pattern", err: err}
		}
		return rx.MatchString, nil
	}
	var negErr *NegExtGlobError
	if !errors.As(err, &negErr) {
		return nil, err
	}
	root, err := parseExtPattern(pat, mode)
	if err != nil {
		return nil, err
	}
	return func(name string) bool {
		return newExtMatcher(mode, name).matches(root)
	}, nil
}

type extTokenKind uint8

const (
	extLiteral extTokenKind = iota
	extAnyOne
	extAnyMany
	extCharClass
	extGroup
)

type extSeq struct {
	id     int
	tokens []extToken
}

type extToken struct {
	kind  extTokenKind
	lit   string
	class *regexp.Regexp
	op    byte
	alts  []*extSeq
}

type extParser struct {
	pat    string
	mode   Mode
	nextID int
}

func parseExtPattern(pat string, mode Mode) (*extSeq, error) {
	p := &extParser{
		pat:    pat,
		mode:   mode,
		nextID: 1,
	}
	seq, i, err := p.parseSeq(0, "")
	if err != nil {
		return nil, err
	}
	if i != len(pat) {
		return nil, &SyntaxError{msg: "unexpected trailing pattern data"}
	}
	return seq, nil
}

func (p *extParser) parseSeq(i int, stops string) (*extSeq, int, error) {
	seq := &extSeq{id: p.nextID}
	p.nextID++
	var lit strings.Builder
	flushLit := func() {
		if lit.Len() == 0 {
			return
		}
		seq.tokens = append(seq.tokens, extToken{kind: extLiteral, lit: lit.String()})
		lit.Reset()
	}
	for i < len(p.pat) {
		c := p.pat[i]
		if strings.IndexByte(stops, c) >= 0 {
			break
		}
		if p.mode&ExtendedOperators != 0 && i+1 < len(p.pat) && p.pat[i+1] == '(' && strings.ContainsRune("!?*+@", rune(c)) {
			flushLit()
			tok, next, err := p.parseGroup(i)
			if errors.Is(err, errUnterminatedExtGlob) {
				// Like bash, match a group without its closing
				// parenthesis, and everything after it, literally.
				lit.WriteString(p.pat[i:])
				i = len(p.pat)
				break
			}
			if err != nil {
				return nil, 0, err
			}
			seq.tokens = append(seq.tokens, tok)
			i = next
			continue
		}
		switch c {
		case '\\':
			if i+1 >= len(p.pat) {
				return nil, 0, &SyntaxError{msg: `\ at end of pattern`}
			}
			_, size := utf8.DecodeRuneInString(p.pat[i+1:])
			lit.WriteString(p.pat[i+1 : i+1+size])
			i += 1 + size
		case '*':
			flushLit()
			// Collapse "**" unless the second star opens an extglob
			// group, as in a**(b).
			if i+1 < len(p.pat) && p.pat[i+1] == '*' &&
				(p.mode&ExtendedOperators == 0 || i+2 >= len(p.pat) || p.pat[i+2] != '(') {
				i++
			}
			seq.tokens = append(seq.tokens, extToken{kind: extAnyMany})
			i++
		case '?':
			flushLit()
			seq.tokens = append(seq.tokens, extToken{kind: extAnyOne})
			i++
		case '[':
			flushLit()
			raw, next, err := scanCharClassToken(p.pat, i)
			if err != nil {
				return nil, 0, err
			}
			expr := "^" + charClassRegexp(raw) + "$"
			if p.mode&NoGlobCase != 0 {
				expr = "(?i)" + expr
			}
			class, err := regexp.Compile(expr)
			if err != nil {
				return nil, 0, err
			}
			seq.tokens = append(seq.tokens, extToken{kind: extCharClass, class: class})
			i = next
		default:
			_, size := utf8.DecodeRuneInString(p.pat[i:])
			if size <= 0 {
				size = 1
			}
			lit.WriteString(p.pat[i : i+size])
			i += size
		}
	}
	flushLit()
	return seq, i, nil
}

var errUnterminatedExtGlob = &SyntaxError{msg: "unterminated extglob"}

func (p *extParser) parseGroup(i int) (extToken, int, error) {
	op := p.pat[i]
	i += 2 // skip op + '('
	var alts []*extSeq
	for {
		alt, next, err := p.parseSeq(i, "|)")
		if err != nil {
			return extToken{}, 0, err
		}
		alts = append(alts, alt)
		if next >= len(p.pat) {
			return extToken{}, 0, errUnterminatedExtGlob
		}
		switch p.pat[next] {
		case '|':
			i = next + 1
		case ')':
			return extToken{kind: extGroup, op: op, alts: alts}, next + 1, nil
		default:
			return extToken{}, 0, errUnterminatedExtGlob
		}
	}
}

func scanCharClassToken(pat string, i int) (string, int, error) {
	if i >= len(pat) || pat[i] != '[' {
		return "", 0, &SyntaxError{msg: "expected ["}
	}
	j := i + 1
	if j >= len(pat) {
		return "", 0, &SyntaxError{msg: "[ was not matched with a closing ]"}
	}
	if pat[j] == '!' || pat[j] == '^' {
		j++
	}
	if j < len(pat) && pat[j] == ']' {
		j++
	}
	for j < len(pat) {
		switch pat[j] {
		case '\\':
			j += 2
			continue
		case '[':
			if name, consumed, err := charClass(pat[j+1:]); err != nil {
				return "", 0, err
			} else if name != "" {
				j += 1 + consumed
				continue
			}
		case ']':
			return pat[i : j+1], j + 1, nil
		}
		j++
	}
	return "", 0, &SyntaxError{msg: "[ was not matched with a closing ]"}
}

// extMatcher simulates a parsed pattern forward over name, tracking the set
// of byte offsets each token can end at. Every token step is linear in the
// name, so patterns without groups match in O(tokens*len(name)).
type extMatcher struct {
	mode Mode
	name string
	memo map[extMemoKey][]int
}

type extMemoKey struct {
	seqID int
	start int
}

func newExtMatcher(mode Mode, name string) *extMatcher {
	return &extMatcher{mode: mode, name: name, memo: make(map[extMemoKey][]int)}
}

func (m *extMatcher) matches(root *extSeq) bool {
	set := make([]bool, len(m.name)+1)
	set[0] = true
	return m.run(root, set)[len(m.name)]
}

// seqEnds returns the sorted offsets at which seq can finish when it starts
// matching at start.
func (m *extMatcher) seqEnds(seq *extSeq, start int) []int {
	key := extMemoKey{seqID: seq.id, start: start}
	if cached, ok := m.memo[key]; ok {
		return cached
	}
	set := make([]bool, len(m.name)+1)
	set[start] = true
	var out []int
	for i, ok := range m.run(seq, set) {
		if ok {
			out = append(out, i)
		}
	}
	m.memo[key] = out
	return out
}

// run advances the start offsets in set through every token of seq and
// returns the set of offsets where seq can end.
func (m *extMatcher) run(seq *extSeq, set []bool) []bool {
	for _, tok := range seq.tokens {
		next := make([]bool, len(set))
		any := false
		mark := func(i int) {
			next[i] = true
			any = true
		}
		switch tok.kind {
		case extLiteral:
			for ni, ok := range set {
				if ok {
					if end, ok := m.matchLiteral(tok.lit, m.name, ni); ok {
						mark(end)
					}
				}
			}
		case extAnyOne:
			for ni, ok := range set {
				if ok {
					if end, ok := m.matchSingle(m.name, ni, true); ok {
						mark(end)
					}
				}
			}
		case extCharClass:
			for ni, ok := range set {
				if ok {
					if end, ok := m.matchCharClass(tok.class, m.name, ni); ok {
						mark(end)
					}
				}
			}
		case extAnyMany:
			for ni, ok := range set {
				// A start already reached by an earlier star run would
				// only re-mark a subset of that run.
				if !ok || next[ni] {
					continue
				}
				for _, end := range m.candidateEnds(m.name, ni, true) {
					mark(end)
				}
			}
		case extGroup:
			m.runGroup(tok, set, mark)
		}
		if !any {
			return next
		}
		set = next
	}
	return set
}

func (m *extMatcher) runGroup(tok extToken, set []bool, mark func(int)) {
	switch tok.op {
	case '@', '?':
		for ni, ok := range set {
			if !ok {
				continue
			}
			if tok.op == '?' {
				mark(ni)
			}
			for _, end := range m.groupOnce(tok, ni) {
				mark(end)
			}
		}
	case '*', '+':
		seen := make([]bool, len(set))
		var work []int
		for ni, ok := range set {
			if !ok {
				continue
			}
			if tok.op == '*' {
				mark(ni)
			}
			work = append(work, ni)
		}
		for len(work) > 0 {
			ni := work[len(work)-1]
			work = work[:len(work)-1]
			for _, end := range m.groupOnce(tok, ni) {
				if end == ni || seen[end] {
					continue
				}
				seen[end] = true
				mark(end)
				work = append(work, end)
			}
		}
	case '!':
		for ni, ok := range set {
			if !ok {
				continue
			}
			excluded := m.groupOnce(tok, ni)
			for _, end := range m.candidateEnds(m.name, ni, true) {
				if _, found := slices.BinarySearch(excluded, end); !found {
					mark(end)
				}
			}
		}
	}
}

// groupOnce returns the sorted offsets at which one alternative of tok can
// end when it starts at start.
func (m *extMatcher) groupOnce(tok extToken, start int) []int {
	if len(tok.alts) == 1 {
		return m.seqEnds(tok.alts[0], start)
	}
	var out []int
	for _, alt := range tok.alts {
		out = append(out, m.seqEnds(alt, start)...)
	}
	return uniqSortedInts(out)
}

func (m *extMatcher) matchLiteral(lit, name string, ni int) (int, bool) {
	if ni+len(lit) > len(name) {
		return 0, false
	}
	part := name[ni : ni+len(lit)]
	if m.mode&NoGlobCase != 0 {
		if !strings.EqualFold(part, lit) {
			return 0, false
		}
	} else if part != lit {
		return 0, false
	}
	return ni + len(lit), true
}

func (m *extMatcher) matchSingle(name string, ni int, respectLeadingDot bool) (int, bool) {
	if ni >= len(name) {
		return 0, false
	}
	r, size := utf8.DecodeRuneInString(name[ni:])
	if size == 0 {
		return 0, false
	}
	if m.mode&Filenames != 0 && r == '/' {
		return 0, false
	}
	if respectLeadingDot && m.leadingDotBlocked(name, ni) && r == '.' {
		return 0, false
	}
	return ni + size, true
}

func (m *extMatcher) matchCharClass(class *regexp.Regexp, name string, ni int) (int, bool) {
	end, ok := m.matchSingle(name, ni, true)
	if !ok {
		return 0, false
	}
	return end, class.MatchString(name[ni:end])
}

func (m *extMatcher) candidateEnds(name string, ni int, respectLeadingDot bool) []int {
	ends := []int{ni}
	if ni >= len(name) {
		return ends
	}
	if respectLeadingDot && m.leadingDotBlocked(name, ni) && name[ni] == '.' {
		return ends
	}
	for i := ni; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if size == 0 {
			break
		}
		if m.mode&Filenames != 0 && r == '/' {
			break
		}
		i += size
		ends = append(ends, i)
	}
	return ends
}

func (m *extMatcher) leadingDotBlocked(name string, ni int) bool {
	if m.mode&Filenames == 0 || m.mode&GlobLeadingDot != 0 || ni >= len(name) || name[ni] != '.' {
		return false
	}
	return ni == 0 || name[ni-1] == '/'
}

func uniqSortedInts(values []int) []int {
	if len(values) < 2 {
		return values
	}
	slices.Sort(values)
	return slices.Compact(values)
}

// charClassRegexp converts a bracket expression scanned by
// scanCharClassToken into Go regexp syntax: a leading ! negates like ^, and
// backslash escapes stay literal rather than becoming regexp escapes like \w.
func charClassRegexp(raw string) string {
	var sb strings.Builder
	sb.WriteByte('[')
	i := 1
	if i < len(raw) && raw[i] == '!' {
		sb.WriteByte('^')
		i++
	}
	for i < len(raw) {
		if raw[i] == '\\' && i+1 < len(raw) {
			_, size := utf8.DecodeRuneInString(raw[i+1:])
			sb.WriteString(quoteClassChar(raw[i+1 : i+1+size]))
			i += 1 + size
			continue
		}
		sb.WriteByte(raw[i])
		i++
	}
	return sb.String()
}
