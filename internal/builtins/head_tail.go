package builtins

import (
	"bytes"
	"math"
	"strings"
)

func isDecimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func parseTailCount(value string, allowFromLine bool) (count int, fromLine bool, err error) {
	fromLine = false
	if allowFromLine && strings.HasPrefix(value, "+") {
		fromLine = true
		value = strings.TrimPrefix(value, "+")
	} else if trimmed, ok := strings.CutPrefix(value, "-"); ok {
		value = trimmed
	}

	parsed, err := parseHeadUnsignedSize(value)
	if err != nil {
		return 0, false, err
	}
	if parsed > uint64(math.MaxInt) {
		return math.MaxInt, fromLine, nil
	}
	return int(parsed), fromLine, nil
}

func splitLines(data []byte) [][]byte {
	return splitDelimitedRecords(data, '\n')
}

func splitDelimitedRecords(data []byte, delim byte) [][]byte {
	if len(data) == 0 {
		return [][]byte{}
	}
	lines := bytes.SplitAfter(data, []byte{delim})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lastDelimitedRecords(data []byte, count int, delim byte) []byte {
	if count <= 0 {
		return nil
	}
	lines := splitDelimitedRecords(data, delim)
	if count > len(lines) {
		count = len(lines)
	}
	return bytes.Join(lines[len(lines)-count:], nil)
}

func delimitedRecordsFrom(data []byte, start int, delim byte) []byte {
	if start <= 1 {
		return data
	}
	lines := splitDelimitedRecords(data, delim)
	if start > len(lines) {
		return nil
	}
	return bytes.Join(lines[start-1:], nil)
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

func bytesFrom(data []byte, start int) []byte {
	if start <= 1 {
		return append([]byte(nil), data...)
	}
	if start > len(data) {
		return nil
	}
	return append([]byte(nil), data[start-1:]...)
}
