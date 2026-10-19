package builtins

import (
	"errors"
	"math"
	"testing"
)

// shufCountingRNG is a deterministic source for iterator tests.
type shufCountingRNG struct{ state uint64 }

func (r *shufCountingRNG) generateAtMost(n uint64) (uint64, error) {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	if n == math.MaxUint64 {
		return r.state, nil
	}
	return r.state % (n + 1), nil
}

func (r *shufCountingRNG) Close() error { return nil }

func TestShufWideRangeStaysSparse(t *testing.T) {
	t.Parallel()

	for _, end := range []uint64{2_000_000_000, math.MaxInt64, math.MaxUint64} {
		iter := newShufNonrepeatingRangeIterator(shufInputRange{start: 0, end: end}, &shufCountingRNG{}, nil)
		seen := make(map[uint64]struct{})
		for range 4 * shufDefaultSparseCapacity {
			value, ok, err := iter.Next()
			if err != nil || !ok {
				t.Fatalf("end %d: Next() = %d, %v, %v", end, value, ok, err)
			}
			if _, dup := seen[value]; dup {
				t.Fatalf("end %d: duplicate value %d", end, value)
			}
			seen[value] = struct{}{}
		}
		if iter.full != nil {
			t.Fatalf("end %d: iterator materialized %d items, want sparse", end, cap(iter.full))
		}
	}
}

func TestShufSparseRangeStopsAtEntryLimit(t *testing.T) {
	t.Parallel()

	iter := newShufNonrepeatingRangeIterator(shufInputRange{start: 0, end: math.MaxUint64}, &shufCountingRNG{}, nil)
	iter.sparseValues = make(map[uint64]uint64, shufSparseMaxEntries)
	for i := range uint64(shufSparseMaxEntries) {
		iter.sparseValues[math.MaxUint64-i] = i
	}
	if _, _, err := iter.Next(); !errors.Is(err, errShufSparseRangeExhausted) {
		t.Fatalf("Next() error = %v, want errShufSparseRangeExhausted", err)
	}
}

func TestShufSparseRangePromotesOnceSmall(t *testing.T) {
	t.Parallel()

	const span = shufFullRangeMaxItems + 2*shufDefaultSparseCapacity
	iter := newShufNonrepeatingRangeIterator(shufInputRange{start: 0, end: span}, &shufCountingRNG{}, nil)
	for iter.full == nil {
		if _, ok, err := iter.Next(); err != nil || !ok {
			t.Fatalf("Next() = %v, %v before promotion", ok, err)
		}
	}
	if got := len(iter.full); got > shufFullRangeMaxItems {
		t.Fatalf("promoted %d items, want at most %d", got, shufFullRangeMaxItems)
	}
}
