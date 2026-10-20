package syntax

import (
	"hash/maphash"
	"io"
)

// rawPartCache memoizes parseRawWordPart for one parse, shared by every
// sub-parser that parseRawWordPart starts beneath it.
//
// Pattern parsing scans the same text several times: to find where a case or
// [[ ]] pattern ends, to look for invalid operators, to split it at "|", and
// finally to build it, and each pass re-parses every shell part such as
// "$(...)" with a fresh sub-parser. When that shell part holds another pattern
// the passes multiply with each level, so nesting such case statements took
// exponential time. With the cache, each part is parsed once per distinct
// input and later passes reuse the result.
//
// The callers hand in different views of the same source: one may stop where
// the pattern ends while another runs to the end of the buffer, and some of
// the sub-parser's output depends on what follows the part. An entry is only
// reused when the new input holds exactly the bytes the sub-parser read for
// it, so a hit returns what a fresh parse would have.
type rawPartCache struct {
	seed    maphash.Seed
	entries map[rawPartKey][]*rawPartEntry
}

type rawPartKey struct {
	pos  Pos
	lang LangVariant
}

type rawPartEntry struct {
	// read is how many bytes of its input the sub-parser read, and readHash
	// their hash. sawEOF reports that it also saw the input end there.
	read     int
	readHash uint64
	sawEOF   bool

	part     WordPart
	consumed int
	ok       bool

	// The sub-parser's nesting state, so that a hit leaves its owner as a
	// fresh parse would. A parse does not otherwise depend on the depth it
	// starts at, so an entry serves any base that cannot reach the limit.
	base          int
	nestPeak      int
	nestHigh      int
	chainLinks    int
	nestingFailed bool
	err           error
}

func newRawPartCache() *rawPartCache {
	return &rawPartCache{
		seed:    maphash.MakeSeed(),
		entries: make(map[rawPartKey][]*rawPartEntry),
	}
}

// lookup returns an entry that parsing raw at base would reproduce exactly,
// for an owner whose nesting depth is depth, or nil.
func (c *rawPartCache) lookup(raw string, base Pos, lang LangVariant, depth int) *rawPartEntry {
	for _, e := range c.entries[rawPartKey{pos: base, lang: lang}] {
		if e.sawEOF && len(raw) != e.read || len(raw) < e.read {
			continue
		}
		if e.nestingFailed {
			// Keep the error at the position a fresh parse reports.
			if depth != e.base {
				continue
			}
		} else if depth+e.nestHigh-e.base > MaxNestingDepth {
			continue
		}
		if maphash.String(c.seed, raw[:e.read]) == e.readHash {
			return e
		}
	}
	return nil
}

func (c *rawPartCache) store(raw string, base Pos, lang LangVariant, e *rawPartEntry) {
	e.readHash = maphash.String(c.seed, raw[:e.read])
	key := rawPartKey{pos: base, lang: lang}
	c.entries[key] = append(c.entries[key], e)
}

// replay applies the nesting state a cached sub-parse left behind to owner,
// shifted to the depth owner is at now.
func (e *rawPartEntry) replay(owner *Parser) {
	shift := func(v int) int {
		if v == 0 {
			return 0
		}
		return v - e.base + owner.nestDepth
	}
	owner.nestPeak = max(owner.nestPeak, shift(e.nestPeak))
	owner.nestHigh = max(owner.nestHigh, shift(e.nestHigh))
	owner.chainLinks += e.chainLinks
	if e.nestingFailed {
		owner.errPass(e.err)
		owner.nestingFailed = true
	}
}

// rawPartReader is a strings.Reader that records how much of its input a
// parser read and whether the parser saw it end.
type rawPartReader struct {
	s      string
	i      int
	sawEOF bool

	// offs is the source offset of s. ahead is how much of s the parser
	// looked at without reading it; see [Parser.rawSourceFrom].
	offs  uint
	ahead int

	// cut reports that s may be cut off before the input ends; see
	// [Parser.inputCut].
	cut bool
}

func (r *rawPartReader) Read(b []byte) (int, error) {
	if r.i >= len(r.s) {
		r.sawEOF = true
		return 0, io.EOF
	}
	n := copy(b, r.s[r.i:])
	r.i += n
	return n, nil
}

// Len implements lenReader.
func (r *rawPartReader) Len() int { return len(r.s) - r.i }
