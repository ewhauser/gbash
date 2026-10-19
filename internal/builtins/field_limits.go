package builtins

import "math"

// defaultMaxFieldBytes is the padded-field cap used when an invocation carries
// no MaxVariableBytes limit. It matches the runtime's default variable limit.
const defaultMaxFieldBytes = 32 << 20

// maxFieldBytes reports the largest padded field, line width, or repeat count
// a text builtin may build from a user-supplied width. It follows printf's
// MaxVariableBytes pre-check so that widths such as `nl -w 4294967296` fail
// before allocating instead of exhausting memory.
func maxFieldBytes(inv *Invocation) int64 {
	if inv == nil || inv.Limits.MaxVariableBytes <= 0 || inv.Limits.MaxVariableBytes == math.MaxInt64 {
		return defaultMaxFieldBytes
	}
	return inv.Limits.MaxVariableBytes
}

// fieldWidthTooLarge reports whether width exceeds the invocation's padded
// field cap. Negative widths are treated as overflowed values.
func fieldWidthTooLarge(inv *Invocation, width int64) bool {
	return width < 0 || width > maxFieldBytes(inv)
}

// fieldWidthError is the shared diagnostic for widths rejected by
// fieldWidthTooLarge.
func fieldWidthError(inv *Invocation, name string) error {
	return exitf(inv, 1, "%s: field width too large (limit %d bytes), increase policy.Limits.MaxVariableBytes", name, maxFieldBytes(inv))
}

// checkFieldWidth returns fieldWidthError when width exceeds the padded field
// cap and nil otherwise.
func checkFieldWidth(inv *Invocation, name string, width int64) error {
	if fieldWidthTooLarge(inv, width) {
		return fieldWidthError(inv, name)
	}
	return nil
}
