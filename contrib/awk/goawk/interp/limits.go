// Memory bounds for strings and arrays (Config.MaxStringBytes and
// Config.MaxArrayBytes).

package interp

import "math"

const (
	// Estimated per-element cost of an array entry beyond its key and string
	// value bytes: the map slot (key and value headers), control bytes, and
	// slack from the map's growth policy.
	arrayEntryOverhead = 96

	// Go's fmt package refuses widths and precisions above this value, so no
	// single printf verb pads by more than it.
	maxFormatPad = 1e6

	// Upper bound on the formatted size of one numeric printf argument
	// before padding: %f of the largest float64 is about 310 digits.
	maxFormattedNumber = 400
)

func (p *interp) stringLimitError() error {
	return newError("string exceeds the %d-byte memory limit", p.maxStringBytes)
}

// checkStringLen reports an error if a string of n bytes would exceed
// MaxStringBytes. Callers check before building the string.
func (p *interp) checkStringLen(n int) error {
	if p.maxStringBytes > 0 && n > p.maxStringBytes {
		return p.stringLimitError()
	}
	return nil
}

func (p *interp) concat(l, r string) (string, error) {
	if err := p.checkStringLen(len(l) + len(r)); err != nil {
		return "", err
	}
	return l + r, nil
}

// checkJoinLen reports an error if joining parts with sep would exceed
// MaxStringBytes.
func (p *interp) checkJoinLen(parts []string, sep string) error {
	if p.maxStringBytes <= 0 || len(parts) == 0 {
		return nil
	}
	n := (len(parts) - 1) * len(sep)
	for _, part := range parts {
		n += len(part)
	}
	return p.checkStringLen(n)
}

// checkFieldsJoinLen reports an error if rebuilding $0 from the current
// fields would exceed MaxStringBytes.
func (p *interp) checkFieldsJoinLen() error {
	if p.maxStringBytes <= 0 {
		return nil
	}
	if p.outputMode != CSVMode && p.outputMode != TSVMode {
		return p.checkJoinLen(p.fields, p.outputFieldSep)
	}
	// Quoting at most doubles a field and adds its surrounding quotes and
	// separator.
	n := 0
	for _, field := range p.fields {
		n += 2*len(field) + 3
	}
	return p.checkStringLen(n)
}

func arrayEntryBytes(key string, v value) int64 {
	return arrayEntryOverhead + int64(len(key)) + int64(len(v.s))
}

func arrayBytes(array map[string]value) int64 {
	var n int64
	for k, v := range array {
		n += arrayEntryBytes(k, v)
	}
	return n
}

// growArrays charges delta bytes against MaxArrayBytes.
func (p *interp) growArrays(delta int64) error {
	if p.maxArrayBytes <= 0 {
		return nil
	}
	if delta > 0 && p.arrayBytes+delta > p.maxArrayBytes {
		return newError("arrays exceed the %d-byte memory limit", p.maxArrayBytes)
	}
	p.arrayBytes += delta
	return nil
}

// releaseArray credits back the bytes of an array that is being dropped.
func (p *interp) releaseArray(array map[string]value) {
	if p.maxArrayBytes > 0 {
		p.arrayBytes -= arrayBytes(array)
	}
}

// arraySet is array[key] = v, charged against MaxArrayBytes.
func (p *interp) arraySet(array map[string]value, key string, v value) error {
	if p.maxArrayBytes > 0 {
		delta := arrayEntryBytes(key, v)
		if old, ok := array[key]; ok {
			delta -= arrayEntryBytes(key, old)
		}
		if err := p.growArrays(delta); err != nil {
			return err
		}
	}
	array[key] = v
	return nil
}

// arrayGet fetches the value at the given index from array. This handles the
// strange POSIX behavior of creating a null entry for non-existent array
// elements. Per the POSIX spec, "Any other reference to a nonexistent array
// element [apart from "in" expressions] shall automatically create it."
func (p *interp) arrayGet(array map[string]value, index string) (value, error) {
	v, ok := array[index]
	if !ok {
		if err := p.growArrays(arrayEntryBytes(index, v)); err != nil {
			return value{}, err
		}
		array[index] = v
	}
	return v, nil
}

func (p *interp) arrayDelete(array map[string]value, key string) {
	if p.maxArrayBytes > 0 {
		old, ok := array[key]
		if !ok {
			return
		}
		p.arrayBytes -= arrayEntryBytes(key, old)
	}
	delete(array, key)
}

func (p *interp) arrayClear(array map[string]value) {
	p.releaseArray(array)
	clear(array)
}

// formatPadBound returns an upper bound on the padding a printf width or
// precision written as a decimal literal can add.
func formatPadBound(n int) int {
	return min(n, maxFormatPad)
}

// starPadBound is formatPadBound for a width or precision taken from a "*"
// argument.
func starPadBound(f float64) int {
	f = math.Abs(f)
	if f > maxFormatPad || math.IsNaN(f) {
		return maxFormatPad
	}
	return int(f)
}
