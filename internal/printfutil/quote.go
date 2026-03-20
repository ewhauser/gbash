package printfutil

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ewhauser/gbash/internal/shell/syntax"
)

func quoteShell(s string) string {
	if s == "" {
		return "''"
	}
	if needsANSIQuote(s) {
		return quoteANSI(s)
	}
	if isSafeRawWord(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if needsBackslashEscape(r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func needsANSIQuote(s string) bool {
	for s != "" {
		r, size := utf8.DecodeRuneInString(s)
		switch {
		case r == utf8.RuneError && size == 1:
			return true
		case r == '\n' || r == '\r' || r == '\t' || r == '\v' || r == '\f' || r == '\a' || r == '\b':
			return true
		case !unicode.IsPrint(r):
			return true
		}
		s = s[size:]
	}
	return false
}

func isSafeRawWord(s string) bool {
	if syntax.IsKeyword(s) {
		return false
	}
	for _, r := range s {
		if needsBackslashEscape(r) {
			return false
		}
	}
	return true
}

func needsBackslashEscape(r rune) bool {
	switch r {
	case ' ', '!', '"', '#', '$', '&', '\'', '(', ')', '*', ';', '<', '=', '>', '?', '[', '\\', ']', '`', '{', '|', '}', '~':
		return true
	default:
		return false
	}
}

func quoteANSI(s string) string {
	var b strings.Builder
	b.WriteString("$'")
	for s != "" {
		r, size := utf8.DecodeRuneInString(s)
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, "\\%03o", s[0])
		case r == '\'' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\a':
			b.WriteString(`\a`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\v':
			b.WriteString(`\v`)
		case !unicode.IsPrint(r):
			for _, raw := range []byte(s[:size]) {
				fmt.Fprintf(&b, "\\%03o", raw)
			}
		default:
			b.WriteRune(r)
		}
		s = s[size:]
	}
	b.WriteByte('\'')
	return b.String()
}
