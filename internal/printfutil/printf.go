package printfutil

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	LookupEnv func(name string) (string, bool)
	Now       func() time.Time
	StartTime time.Time
	Dialect   Dialect
}

type Dialect uint8

const (
	DialectShell Dialect = iota
	DialectGNU
)

type Result struct {
	Output      string
	ExitCode    uint8
	Diagnostics []string
	Warnings    []string
}

type formatSpec struct {
	verb             byte
	timeLayout       string
	leftJustify      bool
	forceSign        bool
	spaceSign        bool
	alternate        bool
	zeroPad          bool
	width            int
	widthSet         bool
	widthFromArg     bool
	precision        int
	precisionSet     bool
	precisionFromArg bool
	quoteFlag        bool
}

type formatToken struct {
	literal string
	spec    *formatSpec
	stop    bool
}

type parsedFormat struct {
	tokens    []formatToken
	verbs     int
	hardStop  bool
	diagLines []string
}

type formatter struct {
	opts        Options
	args        []string
	index       int
	out         strings.Builder
	exitCode    uint8
	diagnostics []string
	warnings    []string
}

func Format(format string, args []string, opts Options) Result {
	parsed := parseFormat(format, opts.Dialect)
	f := formatter{
		opts: normalizeOptions(opts),
		args: append([]string(nil), args...),
	}
	for _, diag := range parsed.diagLines {
		f.addDiagnostic(diag)
	}

	for {
		stop := f.run(parsed.tokens)
		if stop || parsed.hardStop || parsed.verbs == 0 || f.index >= len(f.args) {
			break
		}
	}
	if !parsed.hardStop && parsed.verbs == 0 && f.opts.Dialect == DialectGNU && f.index < len(f.args) {
		f.addWarning(fmt.Sprintf("warning: ignoring excess arguments, starting with %s", quoteGNUDiagnosticOperand(f.args[f.index])))
	}

	return Result{
		Output:      f.out.String(),
		ExitCode:    f.exitCode,
		Diagnostics: append([]string(nil), f.diagnostics...),
		Warnings:    append([]string(nil), f.warnings...),
	}
}

func normalizeOptions(opts Options) Options {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func parseFormat(format string, dialect Dialect) parsedFormat {
	parsed := parsedFormat{
		tokens: make([]formatToken, 0, 8),
	}
	var literal strings.Builder
	flushLiteral := func() {
		if literal.Len() == 0 {
			return
		}
		parsed.tokens = append(parsed.tokens, formatToken{literal: literal.String()})
		literal.Reset()
	}

	for i := 0; i < len(format); {
		switch format[i] {
		case '\\':
			text, next, stop, diag := decodeEscape(format, i, escapeModeOuter, dialect)
			if diag != "" {
				parsed.diagLines = append(parsed.diagLines, diag)
				parsed.hardStop = true
				if dialect == DialectGNU {
					literal.WriteString(text)
					flushLiteral()
					return parsed
				}
			}
			literal.WriteString(text)
			i = next
			if stop {
				flushLiteral()
				parsed.tokens = append(parsed.tokens, formatToken{stop: true})
				return parsed
			}
		case '%':
			flushLiteral()
			if i+1 < len(format) && format[i+1] == '%' {
				literal.WriteByte('%')
				i += 2
				continue
			}
			spec, next, diag, hardStop := parseSpec(format, i, dialect)
			if diag != "" {
				parsed.diagLines = append(parsed.diagLines, diag)
				parsed.hardStop = hardStop
				return parsed
			}
			parsed.tokens = append(parsed.tokens, formatToken{spec: spec})
			parsed.verbs++
			i = next
		default:
			literal.WriteByte(format[i])
			i++
		}
	}
	flushLiteral()
	return parsed
}

func parseSpec(format string, start int, dialect Dialect) (*formatSpec, int, string, bool) {
	spec := &formatSpec{}
	i := start + 1

	for i < len(format) {
		switch format[i] {
		case '-':
			spec.leftJustify = true
		case '+':
			spec.forceSign = true
		case ' ':
			spec.spaceSign = true
		case '#':
			spec.alternate = true
		case '0':
			spec.zeroPad = true
		case '\'':
			if dialect == DialectGNU {
				spec.quoteFlag = true
			} else {
				goto width
			}
		default:
			goto width
		}
		i++
	}

width:
	if i < len(format) && format[i] == '*' {
		spec.widthFromArg = true
		spec.widthSet = true
		i++
	} else {
		width, next, ok := readDigits(format, i)
		if ok {
			spec.width = width
			spec.widthSet = true
			i = next
		}
	}

	if i < len(format) && format[i] == '.' {
		i++
		spec.precisionSet = true
		if i < len(format) && format[i] == '*' {
			spec.precisionFromArg = true
			i++
		} else {
			precision, next, ok := readDigits(format, i)
			if ok {
				spec.precision = precision
				i = next
			} else {
				spec.precision = 0
			}
		}
	}

	if dialect == DialectGNU {
		if i < len(format) && format[i] == '(' {
			return nil, i + 1, gnuInvalidConversionSpec(format[start : i+1]), true
		}
		i = skipGNULengthModifier(format, i)
	}

	if i >= len(format) {
		if dialect == DialectGNU {
			return nil, len(format), gnuFormatEndsInPercent(format[start:]), true
		}
		return nil, len(format), fmt.Sprintf("`%s': missing format character", format[start:]), true
	}
	if dialect != DialectGNU && format[i] == '(' {
		end := strings.IndexByte(format[i+1:], ')')
		if end < 0 || i+1+end+1 >= len(format) {
			return nil, len(format), fmt.Sprintf("`%s': missing format character", format[start:]), true
		}
		end += i + 1
		if format[end+1] != 'T' {
			return nil, end + 1, fmt.Sprintf("`%c': invalid format character", format[end+1]), true
		}
		spec.verb = 'T'
		spec.timeLayout = format[i+1 : end]
		return spec, end + 2, "", false
	}
	if isSupportedVerb(format[i], dialect) {
		spec.verb = format[i]
		if dialect == DialectGNU && (spec.verb == 'b' || spec.verb == 'q') && spec.hasGNUUnsupportedFieldParams() {
			return nil, i + 1, gnuInvalidConversionSpec(format[start : i+1]), true
		}
		return spec, i + 1, "", false
	}
	if dialect == DialectGNU {
		return nil, i + 1, gnuInvalidConversionSpec(format[start : i+1]), true
	}
	for j := i + 1; j < len(format); j++ {
		if isSupportedVerb(format[j], dialect) {
			return nil, j, fmt.Sprintf("`%c': invalid format character", format[i]), true
		}
		if format[j] == '\\' || format[j] == '%' {
			break
		}
	}
	return nil, len(format), fmt.Sprintf("`%s': missing format character", format[start:]), true
}

func readDigits(s string, start int) (int, int, bool) {
	end := start
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == start {
		return 0, start, false
	}
	value, _ := strconv.Atoi(s[start:end])
	return value, end, true
}

func isSupportedVerb(ch byte, dialect Dialect) bool {
	switch dialect {
	case DialectGNU:
		return strings.ContainsRune("bqcsdiouxXefFgGEF", rune(ch))
	default:
		return strings.ContainsRune("bqcsdiouxXefFgGET", rune(ch))
	}
}

func (f *formatter) run(tokens []formatToken) bool {
	for _, token := range tokens {
		switch {
		case token.stop:
			return true
		case token.spec == nil:
			f.out.WriteString(token.literal)
		default:
			if f.applySpec(*token.spec) {
				return true
			}
		}
	}
	return false
}

func (f *formatter) nextArg() (string, bool) {
	if f.index >= len(f.args) {
		return "", false
	}
	arg := f.args[f.index]
	f.index++
	return arg, true
}

func (f *formatter) addDiagnostic(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	f.diagnostics = append(f.diagnostics, message)
	if f.exitCode == 0 {
		f.exitCode = 1
	}
}

func (f *formatter) addWarning(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	f.warnings = append(f.warnings, message)
}

func (f *formatter) applySpec(spec formatSpec) bool {
	if spec.widthFromArg {
		arg, present := f.nextArg()
		width, ok, diag := parseWidthArg(arg, present)
		if diag != "" {
			f.addDiagnostic(diag)
		}
		if ok {
			if width < 0 {
				spec.leftJustify = true
				width = -width
			}
			spec.width = width
			spec.widthSet = true
		} else {
			spec.width = 0
			spec.widthSet = true
		}
	}
	if spec.precisionFromArg {
		arg, present := f.nextArg()
		precision, ok, diag := parseWidthArg(arg, present)
		if diag != "" {
			f.addDiagnostic(diag)
		}
		switch {
		case ok && precision >= 0:
			spec.precision = precision
			spec.precisionSet = true
		case ok && precision < 0:
			spec.precisionSet = false
		default:
			spec.precision = 0
			spec.precisionSet = true
		}
	}

	arg, present := f.nextArg()
	switch spec.verb {
	case 's':
		f.out.WriteString(applyStringFormat(arg, spec))
	case 'q':
		f.out.WriteString(applyStringFormat(quoteShell(arg, f.opts.Dialect), spec))
	case 'b':
		decoded, stop, diag := decodeEscapeString(arg, escapeModePercentB, f.opts.Dialect)
		if diag != "" {
			f.addDiagnostic(diag)
		}
		f.out.WriteString(applyStringFormat(decoded, spec))
		return stop
	case 'c':
		var text string
		if present && arg != "" {
			text = string([]byte{arg[0]})
		} else {
			text = string([]byte{0})
		}
		f.out.WriteString(applyStringFormat(text, spec))
	case 'd', 'i':
		text, diag := formatSigned(arg, present, spec)
		if diag != "" {
			f.addDiagnostic(diag)
		}
		f.out.WriteString(text)
	case 'u', 'o', 'x', 'X':
		text, diag := formatUnsigned(arg, present, spec)
		if diag != "" {
			f.addDiagnostic(diag)
		}
		f.out.WriteString(text)
	case 'e', 'E', 'f', 'F', 'g', 'G':
		text, diag := formatFloat(arg, present, spec)
		if diag != "" {
			f.addDiagnostic(diag)
		}
		f.out.WriteString(text)
	case 'T':
		text, diag := formatTime(arg, present, spec, f.opts)
		if diag != "" {
			f.addDiagnostic(diag)
		}
		f.out.WriteString(text)
	}
	return false
}

func (s formatSpec) hasGNUUnsupportedFieldParams() bool {
	return s.leftJustify ||
		s.forceSign ||
		s.spaceSign ||
		s.alternate ||
		s.zeroPad ||
		s.widthSet ||
		s.widthFromArg ||
		s.precisionSet ||
		s.precisionFromArg ||
		s.quoteFlag
}

func gnuInvalidConversionSpec(spec string) string {
	return fmt.Sprintf("%s: invalid conversion specification", spec)
}

func gnuFormatEndsInPercent(spec string) string {
	return fmt.Sprintf("format %s ends in %%", quoteGNUDiagnosticOperand(spec))
}

func quoteGNUDiagnosticOperand(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func skipGNULengthModifier(format string, start int) int {
	if start >= len(format) {
		return start
	}
	switch format[start] {
	case 'h', 'l':
		if start+1 < len(format) && format[start+1] == format[start] {
			return start + 2
		}
		return start + 1
	case 'j', 'z', 't', 'L':
		return start + 1
	default:
		return start
	}
}

func applyStringFormat(value string, spec formatSpec) string {
	if spec.precisionSet && spec.precision < len(value) {
		value = value[:spec.precision]
	}
	if !spec.widthSet || spec.width <= len(value) {
		return value
	}
	pad := " "
	if spec.zeroPad && !spec.leftJustify && runtime.GOOS != "linux" {
		pad = "0"
	}
	padding := strings.Repeat(pad, spec.width-len(value))
	if spec.leftJustify {
		return value + padding
	}
	return padding + value
}
