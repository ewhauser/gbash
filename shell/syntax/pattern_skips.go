package syntax

import "unsafe"

// patternSkips memoizes where the extended globs and bare groups of one
// pattern end. Every group of a pattern is scanned by each group around it,
// and again when its own arms are built, so without the memo nested groups
// took time quadratic in their depth (exponential before scans stopped
// building the arms).
//
// Arms are passed down as substrings of the pattern, so entries are keyed by
// the address of the group's first byte: equal addresses hold the same
// bytes. An entry is reused only when the new text still holds everything
// the scan looked at, and ends at the same place if the scan saw the end. A
// nil *patternSkips memoizes nothing.
type patternSkips struct {
	m map[uintptr]*patternSkip
}

type patternSkip struct {
	// raw keeps the scanned text alive, so that no other text can take its
	// addresses while the entry exists.
	raw string

	ext bool
	end uintptr
	ok  bool

	// far is the address past the last byte the scan looked at, and
	// rawEnd the end of its text, which it saw if sawEnd is set.
	far, rawEnd uintptr
	sawEnd      bool

	// relHigh is how much deeper than its start the scan nested.
	relHigh int
}

func stringAddr(s string) uintptr {
	return uintptr(unsafe.Pointer(unsafe.StringData(s)))
}

func (s *patternSkips) lookup(raw string, start int, ext bool, owner *Parser) (*patternSkip, bool) {
	if s == nil || s.m == nil || len(raw) == 0 {
		return nil, false
	}
	addr := stringAddr(raw)
	e := s.m[addr+uintptr(start)]
	if e == nil || e.ext != ext {
		return nil, false
	}
	rawEnd := addr + uintptr(len(raw))
	if e.sawEnd && rawEnd != e.rawEnd || rawEnd < e.far {
		return nil, false
	}
	if owner != nil && owner.nestDepth+e.relHigh > MaxNestingDepth {
		// A fresh scan would fail here; let it.
		return nil, false
	}
	return e, true
}

// startHigh prepares owner for measuring how deep a scan nests, returning
// what to pass to store.
func (s *patternSkips) startHigh(owner *Parser) int {
	if owner == nil {
		return 0
	}
	high := owner.nestHigh
	owner.nestHigh = owner.nestDepth
	return high
}

func (s *patternSkips) store(raw string, start int, ext bool, end int, ok bool, seen scanSeen, owner *Parser, high int) {
	relHigh := 0
	if owner != nil {
		relHigh = owner.nestHigh - owner.nestDepth
		owner.nestHigh = max(owner.nestHigh, high)
		if owner.nestingFailed {
			return
		}
	}
	if s == nil || len(raw) == 0 {
		return
	}
	if s.m == nil {
		s.m = make(map[uintptr]*patternSkip)
	}
	addr := stringAddr(raw)
	s.m[addr+uintptr(start)] = &patternSkip{
		raw:     raw,
		ext:     ext,
		end:     addr + uintptr(end),
		ok:      ok,
		far:     addr + uintptr(seen.far),
		rawEnd:  addr + uintptr(len(raw)),
		sawEnd:  seen.end,
		relHigh: relHigh,
	}
}

// end returns the entry's end as an index into raw.
func (e *patternSkip) endIn(raw string) int {
	return int(e.end - stringAddr(raw))
}

// seenIn returns what the entry's scan looked at, as indexes into raw.
func (e *patternSkip) seenIn(raw string) scanSeen {
	return scanSeen{far: int(e.far - stringAddr(raw)), end: e.sawEnd}
}

// scanSeen records how much of its text a scan looked at: the bytes before
// index far, and whether it saw the text end.
type scanSeen struct {
	far int
	end bool
}

func (s *scanSeen) upTo(n int) { s.far = max(s.far, n) }

func (s *scanSeen) sawEnd() { s.end = true }

// has reports whether raw has a byte at index i, recording the check.
func (s *scanSeen) has(raw string, i int) bool {
	if i < len(raw) {
		s.upTo(i + 1)
		return true
	}
	s.sawEnd()
	return false
}

func (s *scanSeen) charClassEnd(raw string, start int) int {
	end := patternCharClassEnd(raw, start)
	s.upTo(end)
	if end >= len(raw) {
		s.sawEnd()
	}
	return end
}

// add merges what another scan of the same raw looked at. s may be nil.
func (s *scanSeen) add(other scanSeen) {
	if s == nil {
		return
	}
	s.upTo(other.far)
	s.end = s.end || other.end
}
