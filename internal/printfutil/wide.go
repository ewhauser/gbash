package printfutil

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// fmtMaxWidth is the largest width or precision package fmt honors; past it
// fmt prints %!(NOVERB) or %!(BADPREC) instead of the value.
const fmtMaxWidth = 1_000_000

// formatNumeric renders value with spec's flags, width, and precision like
// fmt.Sprintf(buildNumericFormat(spec, verb), value), but without fmt's 1e6
// ceiling. Callers bound width and precision with formatter.overLimit first.
func formatNumeric(spec *formatSpec, verb byte, value any) string {
	wide := spec.widthSet && spec.width > fmtMaxWidth
	widePrec := spec.precisionSet && spec.precision > fmtMaxWidth
	if !wide && !widePrec {
		return fmt.Sprintf(buildNumericFormat(spec, verb), value)
	}

	inner := *spec
	inner.widthSet = false
	inner.width = 0
	inner.zeroPad = false
	var text string
	if widePrec {
		text = formatWidePrecision(&inner, verb, value)
	} else {
		text = fmt.Sprintf(buildNumericFormat(&inner, verb), value)
	}
	if !spec.widthSet || spec.width <= len(text) {
		return text
	}
	padding := spec.width - len(text)
	switch {
	case spec.leftJustify:
		return text + strings.Repeat(" ", padding)
	case spec.zeroPad && numericZeroPads(spec, value):
		prefixLen := numericPrefixLen(text, verb)
		return text[:prefixLen] + strings.Repeat("0", padding) + text[prefixLen:]
	default:
		return strings.Repeat(" ", padding) + text
	}
}

// numericZeroPads mirrors C: the 0 flag is ignored for integers with an
// explicit precision and for non-finite floats.
func numericZeroPads(spec *formatSpec, value any) bool {
	if f, ok := value.(float64); ok {
		return !math.IsInf(f, 0) && !math.IsNaN(f)
	}
	return !spec.precisionSet
}

// numericPrefixLen reports how many leading bytes of text (a sign, and a 0x
// prefix for alternate hex) zero padding must follow.
func numericPrefixLen(text string, verb byte) int {
	n := 0
	if n < len(text) && (text[n] == '+' || text[n] == '-' || text[n] == ' ') {
		n++
	}
	if (verb == 'x' || verb == 'X') && n+1 < len(text) && text[n] == '0' && (text[n+1] == 'x' || text[n+1] == 'X') {
		n += 2
	}
	return n
}

// formatWidePrecision renders value with a precision fmt would reject.
// spec carries no width.
func formatWidePrecision(spec *formatSpec, verb byte, value any) string {
	switch v := value.(type) {
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			noPrec := *spec
			noPrec.precisionSet = false
			return fmt.Sprintf(buildNumericFormat(&noPrec, verb), v)
		}
		format := verb
		if format == 'F' {
			format = 'f'
		}
		sign := ""
		switch {
		case math.Signbit(v):
			sign = "-"
		case spec.forceSign:
			sign = "+"
		case spec.spaceSign:
			sign = " "
		}
		return sign + strconv.FormatFloat(math.Abs(v), format, spec.precision, 64)
	default:
		// Integers: precision is a minimum digit count, so format the
		// digits without it and zero-extend them.
		noPrec := *spec
		noPrec.precisionSet = false
		if verb == 'o' {
			// The zero padding already supplies octal's leading 0.
			noPrec.alternate = false
		}
		text := fmt.Sprintf(buildNumericFormat(&noPrec, verb), value)
		prefixLen := numericPrefixLen(text, verb)
		digits := text[prefixLen:]
		if len(digits) >= spec.precision {
			return text
		}
		return text[:prefixLen] + strings.Repeat("0", spec.precision-len(digits)) + digits
	}
}
