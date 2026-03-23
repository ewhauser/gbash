package codingtools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	gbfs "github.com/ewhauser/gbash/fs"
)

const contentTypeSniffBytes = 512

func readResponse(ctx context.Context, fsys gbfs.FileSystem, absolutePath string, req ReadRequest, opts TruncationOptions) (ReadResponse, error) {
	file, err := fsys.Open(ctx, absolutePath)
	if err != nil {
		return ReadResponse{}, err
	}
	defer func() { _ = file.Close() }()

	sniff, err := readAtMost(file, contentTypeSniffBytes)
	if err != nil {
		return ReadResponse{}, err
	}

	reader := io.MultiReader(bytes.NewReader(sniff), file)
	if mimeType := detectSupportedImageMimeType(sniff); mimeType != "" {
		buffer, err := readAllReader(reader)
		if err != nil {
			return ReadResponse{}, err
		}
		return ReadResponse{
			Content: []ContentBlock{
				{Type: "text", Text: fmt.Sprintf("Read image file [%s]", mimeType)},
				{Type: "image", Data: base64.StdEncoding.EncodeToString(buffer), MIMEType: mimeType},
			},
		}, nil
	}

	accumulator := newTextReadAccumulator(req, opts)
	if err := streamTextLines(reader, accumulator); err != nil {
		return ReadResponse{}, err
	}
	return accumulator.response()
}

type textReadAccumulator struct {
	startLine  int
	limit      *int
	truncation TruncationOptions

	totalFileLines int
	selectedLines  int
	selectedBytes  int

	outputLines []string
	outputBytes int

	hasMoreAfterSelection bool
	truncated             bool
	truncatedBy           string
	firstLineTooLong      bool
	firstSelectedBytes    int
}

func newTextReadAccumulator(req ReadRequest, opts TruncationOptions) *textReadAccumulator {
	startLine := 0
	if req.Offset != nil {
		startLine = maxInt(0, *req.Offset-1)
	}
	return &textReadAccumulator{
		startLine:  startLine,
		limit:      req.Limit,
		truncation: normalizeTruncationOptions(opts),
	}
}

func (a *textReadAccumulator) shouldBufferCurrentLine() bool {
	lineIndex := a.totalFileLines
	if lineIndex < a.startLine {
		return false
	}
	if a.limit != nil && a.selectedLines >= *a.limit {
		return false
	}
	if a.truncated || len(a.outputLines) >= a.truncation.MaxLines {
		return false
	}
	return a.currentLineBufferLimit() > 0
}

func (a *textReadAccumulator) currentLineBufferLimit() int {
	newlineBytes := 0
	if len(a.outputLines) > 0 {
		newlineBytes = 1
	}
	return maxInt(0, a.truncation.MaxBytes-a.outputBytes-newlineBytes)
}

func (a *textReadAccumulator) consumeLine(line string, lineBytes int) {
	lineIndex := a.totalFileLines
	a.totalFileLines++

	if lineIndex < a.startLine {
		return
	}
	if a.limit != nil && a.selectedLines >= *a.limit {
		a.hasMoreAfterSelection = true
		return
	}

	a.selectedLines++
	if a.selectedLines > 1 {
		a.selectedBytes++
	}

	a.selectedBytes += lineBytes
	if a.selectedLines == 1 {
		a.firstSelectedBytes = lineBytes
	}

	if a.truncated {
		return
	}

	if a.selectedLines == 1 && lineBytes > a.truncation.MaxBytes {
		a.truncated = true
		a.truncatedBy = "bytes"
		a.firstLineTooLong = true
		return
	}

	if len(a.outputLines) >= a.truncation.MaxLines {
		a.truncated = true
		a.truncatedBy = "lines"
		return
	}

	outputLineBytes := lineBytes
	if len(a.outputLines) > 0 {
		outputLineBytes++
	}
	if a.outputBytes+outputLineBytes > a.truncation.MaxBytes {
		a.truncated = true
		a.truncatedBy = "bytes"
		return
	}

	a.outputLines = append(a.outputLines, line)
	a.outputBytes += outputLineBytes
}

func (a *textReadAccumulator) response() (ReadResponse, error) {
	if a.startLine >= a.totalFileLines {
		return ReadResponse{}, fmt.Errorf(
			"offset %d is beyond end of file (%d lines total)",
			a.startLine+1,
			a.totalFileLines,
		)
	}

	outputContent := joinLines(a.outputLines)
	startLineDisplay := a.startLine + 1

	if a.truncated {
		truncation := TruncationResult{
			Content:               outputContent,
			Truncated:             true,
			TruncatedBy:           a.truncatedBy,
			TotalLines:            a.selectedLines,
			TotalBytes:            a.selectedBytes,
			OutputLines:           len(a.outputLines),
			OutputBytes:           len(outputContent),
			LastLinePartial:       false,
			FirstLineExceedsLimit: a.firstLineTooLong,
			MaxLines:              a.truncation.MaxLines,
			MaxBytes:              a.truncation.MaxBytes,
		}

		if a.firstLineTooLong {
			return ReadResponse{
				Content: []ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf(
						"[Line %d is %s and exceeds the %s read limit. This tool does not return partial lines.]",
						startLineDisplay,
						FormatSize(a.firstSelectedBytes),
						FormatSize(truncation.MaxBytes),
					),
				}},
				Details: &ReadDetails{Truncation: &truncation},
			}, nil
		}

		endLineDisplay := startLineDisplay + truncation.OutputLines - 1
		nextOffset := endLineDisplay + 1
		outputText := truncation.Content
		if truncation.TruncatedBy == "lines" {
			outputText += fmt.Sprintf(
				"\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
				startLineDisplay,
				endLineDisplay,
				a.totalFileLines,
				nextOffset,
			)
		} else {
			outputText += fmt.Sprintf(
				"\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
				startLineDisplay,
				endLineDisplay,
				a.totalFileLines,
				FormatSize(truncation.MaxBytes),
				nextOffset,
			)
		}

		return ReadResponse{
			Content: []ContentBlock{{Type: "text", Text: outputText}},
			Details: &ReadDetails{Truncation: &truncation},
		}, nil
	}

	outputText := outputContent
	if a.limit != nil && a.hasMoreAfterSelection {
		remaining := a.totalFileLines - (a.startLine + a.selectedLines)
		nextOffset := a.startLine + a.selectedLines + 1
		outputText += fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", remaining, nextOffset)
	}

	return ReadResponse{
		Content: []ContentBlock{{Type: "text", Text: outputText}},
	}, nil
}

func streamTextLines(reader io.Reader, accumulator *textReadAccumulator) error {
	buffered := bufio.NewReaderSize(reader, 32*1024)
	emitted := false
	lastEndedWithNewline := false
	lineState := newStreamedLine(accumulator)

	for {
		fragment, err := buffered.ReadSlice('\n')
		lineEnded := err == nil && len(fragment) > 0 && fragment[len(fragment)-1] == '\n'
		if lineEnded {
			fragment = fragment[:len(fragment)-1]
		}
		if len(fragment) > 0 {
			lineState.append(fragment)
		}

		switch {
		case lineEnded:
			emitted = true
			lastEndedWithNewline = true
			lineState.finish(accumulator)
			lineState = newStreamedLine(accumulator)
		case errors.Is(err, bufio.ErrBufferFull):
			lastEndedWithNewline = false
		case errors.Is(err, io.EOF):
			if lineState.hasData {
				emitted = true
				lastEndedWithNewline = false
				lineState.finish(accumulator)
			}
			if !emitted || lastEndedWithNewline {
				accumulator.consumeLine("", 0)
			}
			return nil
		case err != nil:
			return err
		}
	}
}

type streamedLine struct {
	buffer       bytes.Buffer
	bufferLimit  int
	byteCount    int
	hasData      bool
	shouldBuffer bool
}

func newStreamedLine(accumulator *textReadAccumulator) *streamedLine {
	bufferLimit := 0
	if accumulator.shouldBufferCurrentLine() {
		bufferLimit = accumulator.currentLineBufferLimit()
	}
	return &streamedLine{
		bufferLimit:  bufferLimit,
		shouldBuffer: bufferLimit > 0,
	}
}

func (l *streamedLine) append(fragment []byte) {
	l.hasData = true
	l.byteCount += len(fragment)
	if !l.shouldBuffer || l.buffer.Len() >= l.bufferLimit {
		return
	}

	remaining := l.bufferLimit - l.buffer.Len()
	if remaining <= 0 {
		return
	}
	if len(fragment) > remaining {
		fragment = fragment[:remaining]
	}
	_, _ = l.buffer.Write(fragment)
}

func (l *streamedLine) finish(accumulator *textReadAccumulator) {
	line := ""
	if l.shouldBuffer && l.buffer.Len() == l.byteCount {
		line = l.buffer.String()
	}
	accumulator.consumeLine(line, l.byteCount)
}

func readAtMost(reader io.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, nil
	}

	output := make([]byte, 0, limit)
	buffer := make([]byte, minInt(32*1024, limit))
	for len(output) < limit {
		readSize := minInt(len(buffer), limit-len(output))
		n, err := reader.Read(buffer[:readSize])
		if n > 0 {
			output = append(output, buffer[:n]...)
		}
		if errors.Is(err, io.EOF) {
			return output, nil
		}
		if err != nil {
			return nil, err
		}
	}
	return output, nil
}

func readAllReader(reader io.Reader) ([]byte, error) {
	var output []byte
	buffer := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			output = append(output, buffer[:n]...)
		}
		if errors.Is(err, io.EOF) {
			return output, nil
		}
		if err != nil {
			return nil, err
		}
	}
}
