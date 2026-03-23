package printfutil

import (
	"fmt"
	"strconv"
	"strings"
)

type escapeMode int

const (
	escapeModeOuter escapeMode = iota
	escapeModePercentB
)

func DecodeEscapes(s string) (decoded string, stop bool, err error) {
	decoded, stop, diag := decodeEscapeString(s, escapeModeOuter, DialectShell)
	if diag == "" {
		return decoded, stop, nil
	}
	return decoded, stop, fmt.Errorf("%s", diag)
}

func decodeEscapeString(s string, mode escapeMode, dialect Dialect) (string, bool, string) {
	var b strings.Builder
	var diags []string
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		text, next, stop, diag := decodeEscape(s, i, mode, dialect)
		if diag != "" {
			diags = append(diags, diag)
			if dialect == DialectGNU {
				b.WriteString(text)
				return b.String(), true, joinDiagnostics(diags)
			}
		}
		b.WriteString(text)
		i = next
		if stop {
			return b.String(), true, joinDiagnostics(diags)
		}
	}
	return b.String(), false, joinDiagnostics(diags)
}

func decodeEscape(s string, start int, mode escapeMode, dialect Dialect) (text string, next int, stop bool, diag string) {
	if start+1 >= len(s) {
		return "\\", start + 1, false, ""
	}
	switch s[start+1] {
	case 'a':
		return "\a", start + 2, false, ""
	case 'b':
		return "\b", start + 2, false, ""
	case 'c':
		return "", start + 2, true, ""
	case 'e':
		if dialect == DialectGNU {
			return "\x1b", start + 2, false, ""
		}
		return "\\e", start + 2, false, ""
	case 'f':
		return "\f", start + 2, false, ""
	case 'n':
		return "\n", start + 2, false, ""
	case 'r':
		return "\r", start + 2, false, ""
	case 't':
		return "\t", start + 2, false, ""
	case 'v':
		return "\v", start + 2, false, ""
	case '\\':
		return "\\", start + 2, false, ""
	case '\'':
		return "'", start + 2, false, ""
	case '"':
		return "\"", start + 2, false, ""
	case '?':
		return "?", start + 2, false, ""
	case 'x':
		return decodeHexEscape(s, start, 2, dialect)
	case 'u':
		return decodeUnicodeEscape(s, start, 4, dialect)
	case 'U':
		return decodeUnicodeEscape(s, start, 8, dialect)
	case '0', '1', '2', '3', '4', '5', '6', '7':
		return decodeOctalEscape(s, start, mode)
	default:
		return "\\" + string(s[start+1]), start + 2, false, ""
	}
}

func decodeHexEscape(s string, start, maxDigits int, dialect Dialect) (string, int, bool, string) {
	end := start + 2
	for end < len(s) && end-(start+2) < maxDigits && IsHexDigit(s[end]) {
		end++
	}
	if end == start+2 {
		if dialect == DialectGNU {
			return "", len(s), false, "missing hexadecimal number in escape"
		}
		return "\\x", start + 2, false, "missing hex digit for \\x"
	}
	value, _ := strconv.ParseUint(s[start+2:end], 16, 8)
	return string([]byte{byte(value)}), end, false, ""
}

func decodeUnicodeEscape(s string, start, maxDigits int, dialect Dialect) (string, int, bool, string) {
	if dialect == DialectGNU {
		end := start + 2 + maxDigits
		if end > len(s) {
			return "", len(s), false, "missing hexadecimal number in escape"
		}
		for i := start + 2; i < end; i++ {
			if !IsHexDigit(s[i]) {
				return "", len(s), false, "missing hexadecimal number in escape"
			}
		}
		value, _ := strconv.ParseUint(s[start+2:end], 16, 32)
		if value > 0x10ffff || value >= 0xd800 && value <= 0xdfff {
			return "", len(s), false, fmt.Sprintf("invalid universal character name %s", s[start:end])
		}
		return encodeCodePoint(uint32(value)), end, false, ""
	}
	end := start + 2
	for end < len(s) && end-(start+2) < maxDigits && IsHexDigit(s[end]) {
		end++
	}
	if end == start+2 {
		if maxDigits == 4 {
			return "\\u", start + 2, false, "missing unicode digit for \\u"
		}
		return "\\U", start + 2, false, "missing unicode digit for \\U"
	}
	value, _ := strconv.ParseUint(s[start+2:end], 16, 32)
	return encodeCodePoint(uint32(value)), end, false, ""
}

func decodeOctalEscape(s string, start int, mode escapeMode) (string, int, bool, string) {
	limit := 3
	if mode == escapeModePercentB && s[start+1] == '0' {
		limit = 4
	}
	end := start + 1
	for end < len(s) && end-start <= limit && s[end] >= '0' && s[end] <= '7' {
		end++
	}
	value, _ := strconv.ParseUint(s[start+1:end], 8, 32)
	if mode == escapeModeOuter && value > 0xff {
		value &= 0xff
	}
	return string([]byte{byte(value)}), end, false, ""
}

func encodeCodePoint(value uint32) string {
	buf := make([]byte, 0, 6)
	switch {
	case value <= 0x7f:
		buf = append(buf, byte(value))
	case value <= 0x7ff:
		buf = append(buf,
			0xc0|byte(value>>6),
			0x80|byte(value&0x3f),
		)
	case value <= 0xffff:
		buf = append(buf,
			0xe0|byte(value>>12),
			0x80|byte((value>>6)&0x3f),
			0x80|byte(value&0x3f),
		)
	default:
		buf = append(buf,
			0xf0|byte(value>>18),
			0x80|byte((value>>12)&0x3f),
			0x80|byte((value>>6)&0x3f),
			0x80|byte(value&0x3f),
		)
	}
	return string(buf)
}

func joinDiagnostics(diags []string) string {
	if len(diags) == 0 {
		return ""
	}
	return strings.Join(diags, "\n")
}

func IsHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}
