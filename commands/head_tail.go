package commands

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type headTailOptions struct {
	lines    int
	bytes    int
	hasBytes bool
	fromLine bool
	quiet    bool
	verbose  bool
	files    []string
}

func parseHeadTailArgs(inv *Invocation, cmdName string, allowFromLine bool) (headTailOptions, error) {
	args := inv.Args
	opts := headTailOptions{lines: 10}

	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "-n" || arg == "--lines":
			if len(args) < 2 {
				return headTailOptions{}, exitf(inv, 1, "%s: missing argument to -n", cmdName)
			}
			count, fromLine, err := parseHeadTailCount(args[1], allowFromLine)
			if err != nil {
				return headTailOptions{}, exitf(inv, 1, "%s: invalid number of lines", cmdName)
			}
			opts.lines = count
			opts.fromLine = fromLine
			args = args[2:]
		case strings.HasPrefix(arg, "--lines="):
			count, fromLine, err := parseHeadTailCount(strings.TrimPrefix(arg, "--lines="), allowFromLine)
			if err != nil {
				return headTailOptions{}, exitf(inv, 1, "%s: invalid number of lines", cmdName)
			}
			opts.lines = count
			opts.fromLine = fromLine
			args = args[1:]
		case arg == "-c" || arg == "--bytes":
			if len(args) < 2 {
				return headTailOptions{}, exitf(inv, 1, "%s: missing argument to -c", cmdName)
			}
			count, err := parseHeadTailNumber(args[1])
			if err != nil {
				return headTailOptions{}, exitf(inv, 1, "%s: invalid number of bytes", cmdName)
			}
			opts.bytes = count
			opts.hasBytes = true
			args = args[2:]
		case strings.HasPrefix(arg, "--bytes="):
			count, err := parseHeadTailNumber(strings.TrimPrefix(arg, "--bytes="))
			if err != nil {
				return headTailOptions{}, exitf(inv, 1, "%s: invalid number of bytes", cmdName)
			}
			opts.bytes = count
			opts.hasBytes = true
			args = args[1:]
		case strings.HasPrefix(arg, "-n"):
			count, fromLine, err := parseHeadTailCount(strings.TrimPrefix(arg, "-n"), allowFromLine)
			if err != nil {
				return headTailOptions{}, exitf(inv, 1, "%s: invalid number of lines", cmdName)
			}
			opts.lines = count
			opts.fromLine = fromLine
			args = args[1:]
		case strings.HasPrefix(arg, "-c"):
			count, err := parseHeadTailNumber(strings.TrimPrefix(arg, "-c"))
			if err != nil {
				return headTailOptions{}, exitf(inv, 1, "%s: invalid number of bytes", cmdName)
			}
			opts.bytes = count
			opts.hasBytes = true
			args = args[1:]
		case arg == "-q" || arg == "--quiet" || arg == "--silent":
			opts.quiet = true
			args = args[1:]
		case arg == "-v" || arg == "--verbose":
			opts.verbose = true
			args = args[1:]
		case len(arg) > 1 && arg[0] == '-' && arg[1] >= '0' && arg[1] <= '9':
			count, err := strconv.Atoi(arg[1:])
			if err != nil {
				return headTailOptions{}, exitf(inv, 1, "%s: invalid number of lines", cmdName)
			}
			opts.lines = count
			args = args[1:]
		case strings.HasPrefix(arg, "-"):
			return headTailOptions{}, exitf(inv, 1, "%s: unsupported flag %s", cmdName, arg)
		default:
			opts.files = append(opts.files, arg)
			args = args[1:]
		}
	}

	return opts, nil
}

func parseHeadTailCount(value string, allowFromLine bool) (count int, fromLine bool, err error) {
	fromLine = false
	if allowFromLine && strings.HasPrefix(value, "+") {
		fromLine = true
		value = strings.TrimPrefix(value, "+")
	}
	count, err = parseHeadTailNumber(value)
	return count, fromLine, err
}

func parseHeadTailNumber(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("invalid count")
	}

	multiplier := int64(1)
	for _, suffix := range []struct {
		token      string
		multiplier int64
	}{
		{"E", 1 << 60},
		{"P", 1 << 50},
		{"T", 1 << 40},
		{"G", 1 << 30},
		{"M", 1 << 20},
		{"K", 1 << 10},
		{"b", 512},
	} {
		if before, ok := strings.CutSuffix(value, suffix.token); ok {
			value = before
			multiplier = suffix.multiplier
			break
		}
	}

	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("invalid count")
	}
	if count > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("invalid count")
	}
	total := count * multiplier
	if total > int64(math.MaxInt) {
		return 0, fmt.Errorf("invalid count")
	}
	return int(total), nil
}

func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := bytes.SplitAfter(data, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lastLines(data []byte, count int) []byte {
	if count <= 0 {
		return nil
	}
	lines := splitLines(data)
	if count > len(lines) {
		count = len(lines)
	}
	return bytes.Join(lines[len(lines)-count:], nil)
}

func linesFrom(data []byte, startLine int) []byte {
	if startLine <= 1 {
		return data
	}
	lines := splitLines(data)
	if startLine > len(lines) {
		return nil
	}
	return bytes.Join(lines[startLine-1:], nil)
}

func lastBytes(data []byte, count int) []byte {
	if count <= 0 {
		return nil
	}
	if count > len(data) {
		count = len(data)
	}
	return append([]byte(nil), data[len(data)-count:]...)
}
