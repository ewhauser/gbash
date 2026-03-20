package printfutil

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"
)

func formatTime(arg string, present bool, spec formatSpec, opts Options) (string, string) {
	when := opts.Now()
	if present {
		parsed := parseInteger(arg, true)
		if parsed.diagnose != "" {
			// Bash's %T doesn't have coverage for invalid operands in our corpus,
			// but staying consistent with numeric conversions is safer.
			sec := int64(0)
			if parsed.value != nil {
				sec = clampSigned64(parsed.value)
			}
			when = time.Unix(sec, 0)
			when = when.In(resolveLocation(opts))
			return applyStringFormat(strftime(spec.timeLayout, when), spec), parsed.diagnose
		}
		if parsed.value != nil {
			when = time.Unix(clampSigned64(parsed.value), 0)
		}
	}
	when = when.In(resolveLocation(opts))
	text := strftime(spec.timeLayout, when)
	if len(text) >= 128 {
		text = ""
	}
	return applyStringFormat(text, spec), ""
}

func clampSigned64(value *big.Int) int64 {
	switch {
	case value == nil:
		return 0
	case value.Cmp(maxInt64Big) > 0:
		return math.MaxInt64
	case value.Cmp(minInt64Big) < 0:
		return math.MinInt64
	default:
		return value.Int64()
	}
}

func resolveLocation(opts Options) *time.Location {
	if opts.LookupEnv != nil {
		if value, ok := opts.LookupEnv("TZ"); ok && strings.TrimSpace(value) != "" {
			if loc, err := time.LoadLocation(value); err == nil {
				return loc
			}
		}
	}
	return time.Local
}

func strftime(layout string, when time.Time) string {
	var b strings.Builder
	for i := 0; i < len(layout); i++ {
		if layout[i] != '%' || i+1 >= len(layout) {
			b.WriteByte(layout[i])
			continue
		}
		i++
		switch layout[i] {
		case '%':
			b.WriteByte('%')
		case 'Y':
			fmt.Fprintf(&b, "%04d", when.Year())
		case 'm':
			fmt.Fprintf(&b, "%02d", int(when.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", when.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", when.Hour())
		case 'M':
			fmt.Fprintf(&b, "%02d", when.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", when.Second())
		default:
			b.WriteByte('%')
			b.WriteByte(layout[i])
		}
	}
	return b.String()
}
