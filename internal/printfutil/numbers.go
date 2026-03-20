package printfutil

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

var (
	maxInt64Big  = big.NewInt(math.MaxInt64)
	minInt64Big  = big.NewInt(math.MinInt64)
	maxUint64Big = new(big.Int).SetUint64(math.MaxUint64)
	twoTo64Big   = new(big.Int).Add(maxUint64Big, big.NewInt(1))
)

type numericParse struct {
	value    *big.Int
	diagnose string
}

func parseWidthArg(arg string, present bool) (int, bool, string) {
	if !present {
		return 0, true, ""
	}
	parsed := parseInteger(arg, true)
	if parsed.diagnose != "" {
		return clampInt(parsed.value), false, parsed.diagnose
	}
	return clampInt(parsed.value), true, ""
}

func clampInt(value *big.Int) int {
	if value == nil {
		return 0
	}
	if value.Cmp(maxInt64Big) > 0 {
		return math.MaxInt
	}
	if value.Cmp(minInt64Big) < 0 {
		return math.MinInt
	}
	return int(value.Int64())
}

func formatSigned(arg string, present bool, spec formatSpec) (string, string) {
	parsed := parseInteger(arg, present)
	value := int64(0)
	diag := parsed.diagnose
	if parsed.value != nil {
		switch {
		case parsed.value.Cmp(maxInt64Big) > 0:
			value = math.MaxInt64
			if diag == "" {
				diag = fmt.Sprintf("%s: Result too large", arg)
			}
		case parsed.value.Cmp(minInt64Big) < 0:
			value = math.MinInt64
			if diag == "" {
				diag = fmt.Sprintf("%s: Result too large", arg)
			}
		default:
			value = parsed.value.Int64()
		}
	}
	return fmt.Sprintf(buildNumericFormat(spec, 'd'), value), diag
}

func formatUnsigned(arg string, present bool, spec formatSpec) (string, string) {
	parsed := parseInteger(arg, present)
	value := uint64(0)
	diag := parsed.diagnose
	if parsed.value != nil {
		switch {
		case parsed.value.Sign() >= 0 && parsed.value.Cmp(maxUint64Big) > 0:
			value = math.MaxUint64
			if diag == "" {
				diag = fmt.Sprintf("%s: Result too large", arg)
			}
		case parsed.value.Sign() < 0:
			abs := new(big.Int).Neg(parsed.value)
			if abs.Cmp(maxUint64Big) > 0 {
				value = math.MaxUint64
				if diag == "" {
					diag = fmt.Sprintf("%s: Result too large", arg)
				}
			} else {
				mod := new(big.Int).Mod(parsed.value, twoTo64Big)
				value = mod.Uint64()
			}
		default:
			value = parsed.value.Uint64()
		}
	}
	verb := spec.verb
	if verb == 'u' {
		verb = 'd'
	}
	return fmt.Sprintf(buildNumericFormat(spec, verb), value), diag
}

func formatFloat(arg string, present bool, spec formatSpec) (string, string) {
	if present {
		if value, ok := parseQuotedCharArg(arg); ok {
			return fmt.Sprintf(buildNumericFormat(spec, spec.verb), float64(value)), ""
		}
	}
	if !present {
		return fmt.Sprintf(buildNumericFormat(spec, spec.verb), 0.0), ""
	}

	trimmed := strings.TrimLeft(arg, " \t\r\n\v\f")
	if trimmed == "" {
		return fmt.Sprintf(buildNumericFormat(spec, spec.verb), 0.0), fmt.Sprintf("%s: invalid number", arg)
	}
	prefix := floatPrefix(trimmed)
	if prefix == "" {
		return fmt.Sprintf(buildNumericFormat(spec, spec.verb), 0.0), fmt.Sprintf("%s: invalid number", arg)
	}
	value, err := strconv.ParseFloat(prefix, 64)
	diag := ""
	if err != nil || prefix != trimmed {
		diag = fmt.Sprintf("%s: invalid number", arg)
	}
	return fmt.Sprintf(buildNumericFormat(spec, spec.verb), value), diag
}

func floatPrefix(s string) string {
	start := 0
	if s != "" && (s[0] == '+' || s[0] == '-') {
		start = 1
	}
	i := start
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return ""
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		expStart := i
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		expDigits := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			expDigits++
		}
		if expDigits == 0 {
			i = expStart
		}
	}
	return s[:i]
}

func buildNumericFormat(spec formatSpec, verb byte) string {
	var b strings.Builder
	b.WriteByte('%')
	if spec.alternate {
		b.WriteByte('#')
	}
	if spec.forceSign {
		b.WriteByte('+')
	}
	if spec.spaceSign {
		b.WriteByte(' ')
	}
	if spec.leftJustify {
		b.WriteByte('-')
	}
	if spec.zeroPad {
		b.WriteByte('0')
	}
	if spec.widthSet {
		b.WriteString(strconv.Itoa(spec.width))
	}
	if spec.precisionSet {
		b.WriteByte('.')
		b.WriteString(strconv.Itoa(spec.precision))
	}
	b.WriteByte(verb)
	return b.String()
}

func parseInteger(arg string, present bool) numericParse {
	if !present {
		return numericParse{value: big.NewInt(0)}
	}
	if value, ok := parseQuotedCharArg(arg); ok {
		return numericParse{value: big.NewInt(value)}
	}

	trimmed := strings.TrimLeft(arg, " \t\r\n\v\f")
	if trimmed == "" {
		return numericParse{
			value:    big.NewInt(0),
			diagnose: fmt.Sprintf("%s: invalid number", arg),
		}
	}

	sign := 1
	switch trimmed[0] {
	case '+':
		trimmed = trimmed[1:]
	case '-':
		sign = -1
		trimmed = trimmed[1:]
	}

	base := 10
	prefixLen := 0
	switch {
	case strings.HasPrefix(trimmed, "0x") || strings.HasPrefix(trimmed, "0X"):
		base = 16
		prefixLen = 2
	case strings.HasPrefix(trimmed, "0"):
		base = 8
		prefixLen = 1
	}

	startDigits := trimmed[prefixLen:]
	endDigits := 0
	for endDigits < len(startDigits) && validDigit(startDigits[endDigits], base) {
		endDigits++
	}

	digits := startDigits[:endDigits]
	if prefixLen == 1 {
		digits = "0" + digits
	}

	rest := startDigits[endDigits:]
	if prefixLen == 2 && digits == "" {
		rest = trimmed[prefixLen:]
	}
	invalid := rest != ""
	if digits == "" {
		invalid = true
		digits = "0"
	}

	value := new(big.Int)
	value.SetString(digits, base)
	if sign < 0 {
		value.Neg(value)
	}
	if invalid {
		return numericParse{
			value:    value,
			diagnose: fmt.Sprintf("%s: invalid number", arg),
		}
	}
	return numericParse{value: value}
}

func validDigit(ch byte, base int) bool {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch-'0') < base
	case ch >= 'a' && ch <= 'f':
		return 10+int(ch-'a') < base
	case ch >= 'A' && ch <= 'F':
		return 10+int(ch-'A') < base
	default:
		return false
	}
}

func parseQuotedCharArg(arg string) (int64, bool) {
	if arg == "" {
		return 0, false
	}
	if arg[0] != '\'' && arg[0] != '"' {
		return 0, false
	}
	if len(arg) == 1 {
		return 0, true
	}
	return int64(decodeShellCharValue([]byte(arg[1:]))), true
}

func decodeShellCharValue(data []byte) uint32 {
	if len(data) == 0 {
		return 0
	}
	if data[0] < 0x80 {
		return uint32(data[0])
	}
	if len(data) >= 2 && data[0] >= 0xc2 && data[0] <= 0xdf && isContinuation(data[1]) {
		value := uint32(data[0]&0x1f)<<6 | uint32(data[1]&0x3f)
		if value < 0x80 {
			return uint32(data[0])
		}
		return value
	}
	if len(data) >= 3 && data[0] >= 0xe0 && data[0] <= 0xef &&
		isContinuation(data[1]) && isContinuation(data[2]) {
		value := uint32(data[0]&0x0f)<<12 | uint32(data[1]&0x3f)<<6 | uint32(data[2]&0x3f)
		if value < 0x800 || (value >= 0xd800 && value <= 0xdfff) {
			return uint32(data[0])
		}
		return value
	}
	if len(data) >= 4 && data[0] >= 0xf0 && data[0] <= 0xf7 &&
		isContinuation(data[1]) && isContinuation(data[2]) && isContinuation(data[3]) {
		value := uint32(data[0]&0x07)<<18 | uint32(data[1]&0x3f)<<12 | uint32(data[2]&0x3f)<<6 | uint32(data[3]&0x3f)
		if value < 0x10000 || value > 0x10ffff {
			return uint32(data[0])
		}
		return value
	}
	return uint32(data[0])
}

func isContinuation(b byte) bool {
	return b&0xc0 == 0x80
}
