package builtins

import "math"

type shufInputRange struct {
	start uint64
	end   uint64
	empty bool
}

type shufNonrepeatingRangeIterator struct {
	rng            shufRandomSource
	remaining      uint64
	limited        bool
	full           []uint64
	sparseStart    uint64
	sparseEnd      uint64
	sparseValues   map[uint64]uint64
	sparseCapacity int
}

func (r shufInputRange) choose(rng shufRandomSource) (uint64, error) {
	value, err := rng.generateAtMost(r.end - r.start)
	if err != nil {
		return 0, err
	}
	return r.start + value, nil
}

func newShufNonrepeatingRangeIterator(inputRange shufInputRange, rng shufRandomSource, headCount *uint64) *shufNonrepeatingRangeIterator {
	iter := &shufNonrepeatingRangeIterator{
		rng:         rng,
		sparseStart: inputRange.start,
		sparseEnd:   inputRange.end,
	}
	if headCount != nil {
		iter.remaining = *headCount
		iter.limited = true
	}

	if inputRange.empty {
		iter.full = []uint64{}
		return iter
	}

	span := inputRange.end - inputRange.start
	if span < shufFullRangeMaxItems {
		length := int(span + 1)
		items := make([]uint64, length)
		for i := range length {
			items[length-1-i] = inputRange.start + uint64(i)
		}
		iter.full = items
		return iter
	}

	capacity := shufDefaultSparseCapacity
	if headCount != nil && *headCount < uint64(capacity) {
		capacity = int(*headCount)
	}
	if capacity < 1 {
		capacity = 1
	}
	iter.sparseCapacity = capacity
	iter.sparseValues = make(map[uint64]uint64, capacity)
	return iter
}

func (it *shufNonrepeatingRangeIterator) Next() (uint64, bool, error) {
	if it.limited && it.remaining == 0 {
		return 0, false, nil
	}
	if len(it.full) == 0 && it.sparseValues == nil {
		return 0, false, nil
	}
	if it.full == nil {
		if it.sparseStart > it.sparseEnd {
			return 0, false, nil
		}
		if len(it.sparseValues) >= it.sparseCapacity {
			it.promoteSparseToFull()
		}
	}

	var (
		value uint64
		err   error
	)
	if it.full != nil {
		if len(it.full) == 0 {
			return 0, false, nil
		}
		value, err = it.nextFull()
	} else {
		value, err = it.nextSparse()
	}
	if err != nil {
		return 0, false, err
	}
	if it.limited {
		it.remaining--
	}
	return value, true, nil
}

func (it *shufNonrepeatingRangeIterator) nextFull() (uint64, error) {
	last := len(it.full) - 1
	other, err := it.rng.generateAtMost(uint64(last))
	if err != nil {
		return 0, err
	}
	otherIndex := len(it.full) - int(other) - 1
	it.full[last], it.full[otherIndex] = it.full[otherIndex], it.full[last]
	value := it.full[last]
	it.full = it.full[:last]
	return value, nil
}

func (it *shufNonrepeatingRangeIterator) nextSparse() (uint64, error) {
	current := it.sparseStart
	value := current
	if mapped, ok := it.sparseValues[current]; ok {
		value = mapped
		delete(it.sparseValues, current)
	}

	other, err := it.rng.generateAtMost(it.sparseEnd - current)
	if err != nil {
		return 0, err
	}
	otherIndex := current + other
	if otherIndex != current {
		replacement := otherIndex
		if mapped, ok := it.sparseValues[otherIndex]; ok {
			replacement = mapped
		}
		it.sparseValues[otherIndex] = value
		value = replacement
	}

	if current == math.MaxUint64 {
		it.sparseValues = nil
		return value, nil
	}
	it.sparseStart++
	return value, nil
}

func (it *shufNonrepeatingRangeIterator) promoteSparseToFull() {
	length := int(it.sparseEnd - it.sparseStart + 1)
	items := make([]uint64, 0, length)
	for value := it.sparseEnd; ; value-- {
		if mapped, ok := it.sparseValues[value]; ok {
			items = append(items, mapped)
		} else {
			items = append(items, value)
		}
		if value == it.sparseStart {
			break
		}
	}
	it.full = items
	it.sparseValues = nil
}
