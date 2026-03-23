package codingtools

import (
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type fuzzyMatchResult struct {
	found          bool
	replaceStart   int
	replaceEnd     int
	usedFuzzyMatch bool
}

func detectLineEnding(content string) string {
	crlfIdx := strings.Index(content, "\r\n")
	lfIdx := strings.Index(content, "\n")
	switch {
	case lfIdx == -1:
		return "\n"
	case crlfIdx == -1:
		return "\n"
	case crlfIdx < lfIdx:
		return "\r\n"
	default:
		return "\n"
	}
}

func normalizeToLF(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

func normalizeForFuzzyMatch(text string) string {
	normalized, _ := normalizeForFuzzyMatchWithSegments(text)
	return normalized
}

func fuzzyFindText(content, oldText string) fuzzyMatchResult {
	exactIndex := strings.Index(content, oldText)
	if exactIndex != -1 {
		return fuzzyMatchResult{
			found:        true,
			replaceStart: exactIndex,
			replaceEnd:   exactIndex + len(oldText),
		}
	}

	fuzzyContent, segments := normalizeForFuzzyMatchWithSegments(content)
	fuzzyOldText := normalizeForFuzzyMatch(oldText)
	fuzzyIndex := strings.Index(fuzzyContent, fuzzyOldText)
	if fuzzyIndex == -1 {
		return fuzzyMatchResult{
			found:        false,
			replaceStart: -1,
			replaceEnd:   -1,
		}
	}

	replaceStart, replaceEnd, ok := mapFuzzyRangeToOriginal(segments, fuzzyIndex, fuzzyIndex+len(fuzzyOldText))
	if !ok {
		return fuzzyMatchResult{
			found:        false,
			replaceStart: -1,
			replaceEnd:   -1,
		}
	}

	return fuzzyMatchResult{
		found:          true,
		replaceStart:   replaceStart,
		replaceEnd:     replaceEnd,
		usedFuzzyMatch: true,
	}
}

func stripBOM(content string) (string, string) {
	if stripped, ok := strings.CutPrefix(content, "\uFEFF"); ok {
		return "\uFEFF", stripped
	}
	return "", content
}

func normalizedOffsetMap(text string) []int {
	offsets := make([]int, 1, len(text)+1)
	offsets[0] = 0
	for i := 0; i < len(text); {
		switch text[i] {
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				i += 2
			} else {
				i++
			}
		default:
			i++
		}
		offsets = append(offsets, i)
	}
	return offsets
}

func mapNormalizedRangeToOriginal(offsets []int, start, end int) (int, int, bool) {
	if start < 0 || end < start || end >= len(offsets) {
		return 0, 0, false
	}
	return offsets[start], offsets[end], true
}

func extractLineEndings(text string) []string {
	endings := make([]string, 0, strings.Count(text, "\n")+strings.Count(text, "\r"))
	for i := 0; i < len(text); {
		switch text[i] {
		case '\r':
			if i+1 < len(text) && text[i+1] == '\n' {
				endings = append(endings, "\r\n")
				i += 2
				continue
			}
			endings = append(endings, "\r")
		case '\n':
			endings = append(endings, "\n")
		}
		i++
	}
	return endings
}

func restoreReplacementLineEndings(normalizedText, originalSegment, fallback string) string {
	if !strings.Contains(normalizedText, "\n") {
		return normalizedText
	}

	lineEndings := extractLineEndings(originalSegment)
	if fallback == "" {
		fallback = "\n"
	}

	var builder strings.Builder
	builder.Grow(len(normalizedText) + len(lineEndings))
	lineEndingIndex := 0
	for i := 0; i < len(normalizedText); i++ {
		if normalizedText[i] != '\n' {
			builder.WriteByte(normalizedText[i])
			continue
		}

		switch {
		case lineEndingIndex < len(lineEndings):
			builder.WriteString(lineEndings[lineEndingIndex])
		case len(lineEndings) > 0:
			builder.WriteString(lineEndings[len(lineEndings)-1])
		default:
			builder.WriteString(fallback)
		}
		lineEndingIndex++
	}
	return builder.String()
}

func generateDiffString(oldContent, newContent string, contextLines int) (string, int) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	maxLineNum := maxInt(len(oldLines), len(newLines))
	lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))

	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}

	firstChangedLine := prefix + 1
	if prefix == len(newLines) && suffix == 0 {
		firstChangedLine = len(newLines)
	}

	startContext := maxInt(0, prefix-contextLines)
	endContext := minInt(suffix, contextLines)

	var output []string
	if startContext > 0 {
		output = append(output, fmt.Sprintf(" %*s ...", lineNumWidth, ""))
	}

	for i := startContext; i < prefix; i++ {
		output = append(output, fmt.Sprintf(" %*d %s", lineNumWidth, i+1, oldLines[i]))
	}

	oldChangedEnd := len(oldLines) - suffix
	newChangedEnd := len(newLines) - suffix
	for i := prefix; i < oldChangedEnd; i++ {
		output = append(output, fmt.Sprintf("-%*d %s", lineNumWidth, i+1, oldLines[i]))
	}
	for i := prefix; i < newChangedEnd; i++ {
		output = append(output, fmt.Sprintf("+%*d %s", lineNumWidth, i+1, newLines[i]))
	}

	for i := range endContext {
		oldIdx := oldChangedEnd + i
		output = append(output, fmt.Sprintf(" %*d %s", lineNumWidth, oldIdx+1, oldLines[oldIdx]))
	}
	if suffix > endContext {
		output = append(output, fmt.Sprintf(" %*s ...", lineNumWidth, ""))
	}

	return strings.Join(output, "\n"), firstChangedLine
}

func countOverlappingOccurrences(text, target string) int {
	if target == "" {
		return 0
	}

	count := 0
	for start := 0; start <= len(text)-len(target); {
		index := strings.Index(text[start:], target)
		if index == -1 {
			return count
		}
		count++
		start += index + 1
	}
	return count
}

type fuzzySegment struct {
	output    string
	origStart int
	origEnd   int
	outStart  int
	outEnd    int
}

var fuzzyMatchReplacer = strings.NewReplacer(
	"\u2018", "'",
	"\u2019", "'",
	"\u201A", "'",
	"\u201B", "'",
	"\u201C", "\"",
	"\u201D", "\"",
	"\u201E", "\"",
	"\u201F", "\"",
	"\u2010", "-",
	"\u2011", "-",
	"\u2012", "-",
	"\u2013", "-",
	"\u2014", "-",
	"\u2015", "-",
	"\u2212", "-",
	"\u00A0", " ",
	"\u2002", " ",
	"\u2003", " ",
	"\u2004", " ",
	"\u2005", " ",
	"\u2006", " ",
	"\u2007", " ",
	"\u2008", " ",
	"\u2009", " ",
	"\u200A", " ",
	"\u202F", " ",
	"\u205F", " ",
	"\u3000", " ",
)

func normalizeForFuzzyMatchWithSegments(text string) (string, []fuzzySegment) {
	lines := strings.Split(text, "\n")
	segments := make([]fuzzySegment, 0, len(lines))
	var builder strings.Builder

	originalOffset := 0
	for lineIndex, line := range lines {
		lineSegments := normalizeLineForFuzzyMatchSegments(line, originalOffset)
		for _, segment := range lineSegments {
			segment.outStart = builder.Len()
			builder.WriteString(segment.output)
			segment.outEnd = builder.Len()
			segments = append(segments, segment)
		}

		originalOffset += len(line)
		if lineIndex < len(lines)-1 {
			newline := fuzzySegment{
				output:    "\n",
				origStart: originalOffset,
				origEnd:   originalOffset + 1,
				outStart:  builder.Len(),
			}
			builder.WriteString("\n")
			newline.outEnd = builder.Len()
			segments = append(segments, newline)
			originalOffset++
		}
	}

	return builder.String(), segments
}

func normalizeLineForFuzzyMatchSegments(line string, baseOffset int) []fuzzySegment {
	segments := make([]fuzzySegment, 0, len(line))
	for start := 0; start < len(line); {
		size := norm.NFKC.NextBoundaryInString(line[start:], true)
		if size <= 0 {
			size = len(line) - start
		}

		chunk := line[start : start+size]
		normalized := fuzzyMatchReplacer.Replace(norm.NFKC.String(chunk))
		if normalized != "" {
			segments = append(segments, fuzzySegment{
				output:    normalized,
				origStart: baseOffset + start,
				origEnd:   baseOffset + start + size,
			})
		}
		start += size
	}

	return trimTrailingFuzzySegments(segments)
}

func trimTrailingFuzzySegments(segments []fuzzySegment) []fuzzySegment {
	for len(segments) > 0 {
		last := len(segments) - 1
		trimmed := strings.TrimRight(segments[last].output, " \t")
		if trimmed == segments[last].output {
			break
		}
		if trimmed == "" {
			segments = segments[:last]
			continue
		}
		segments[last].output = trimmed
		break
	}
	return segments
}

func mapFuzzyRangeToOriginal(segments []fuzzySegment, start, end int) (int, int, bool) {
	if start < 0 || end <= start {
		return 0, 0, false
	}

	var startSegment *fuzzySegment
	var endSegment *fuzzySegment
	for i := range segments {
		segment := &segments[i]
		if startSegment == nil && start >= segment.outStart && start < segment.outEnd {
			startSegment = segment
		}
		if end > segment.outStart && end <= segment.outEnd {
			endSegment = segment
			break
		}
	}
	if startSegment == nil || endSegment == nil {
		return 0, 0, false
	}
	return startSegment.origStart, endSegment.origEnd, true
}
