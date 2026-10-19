package builtins

import (
	"bufio"
	"errors"
	"io"
	"math"

	"github.com/ewhauser/gbash/commands"
)

// maxInputLimit returns inv's MaxFileBytes, or zero when it is unset or
// unlimited.
func maxInputLimit(inv *Invocation) int64 {
	if inv == nil || inv.Limits.MaxFileBytes <= 0 || inv.Limits.MaxFileBytes == math.MaxInt64 {
		return 0
	}
	return inv.Limits.MaxFileBytes
}

// inputTooLargeError matches the diagnostic commands.ReadAll returns when one
// input exceeds MaxFileBytes.
func inputTooLargeError(limit int64) error {
	return &ExitError{Code: 1, Err: commands.Diagnosticf("input exceeds maximum file size of %d bytes", limit)}
}

// boundedLineReader reads delimiter-terminated records like
// bufio.Reader.ReadBytes, but fails once a single record grows past
// MaxFileBytes instead of buffering an endless line such as /dev/zero.
//
// Streaming builtins (uniq, factor, numfmt, tsort, csplit) use it so their
// memory stays bounded by the longest allowed line rather than the input.
type boundedLineReader struct {
	r     *bufio.Reader
	limit int64
}

func newBoundedLineReader(inv *Invocation, r io.Reader) *boundedLineReader {
	return &boundedLineReader{r: bufio.NewReader(r), limit: maxInputLimit(inv)}
}

// ReadLine returns the next record including delim. Like ReadBytes, it
// returns the data read before an error, and io.EOF for a final record
// without delim. A record past the limit returns inputTooLargeError and no
// data.
func (b *boundedLineReader) ReadLine(delim byte) ([]byte, error) {
	var line []byte
	for {
		frag, err := b.r.ReadSlice(delim)
		if b.limit > 0 && int64(len(line))+int64(len(frag)) > b.limit {
			return nil, inputTooLargeError(b.limit)
		}
		line = append(line, frag...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}

// aggregateInputMultiplier scales MaxFileBytes into the cap on the combined
// size of every file operand one invocation buffers (sort, sed, tac, paste,
// rev, column). It leaves room for several full-size files while keeping
// `sort f f f ...` from multiplying one file into gigabytes of memory.
const aggregateInputMultiplier = 4

// maxAggregateInputBytes reports the combined-operand cap for inv, or zero
// when MaxFileBytes is unset or unlimited.
func maxAggregateInputBytes(inv *Invocation) int64 {
	limit := maxInputLimit(inv)
	if limit == 0 {
		return 0
	}
	if limit > math.MaxInt64/aggregateInputMultiplier {
		return math.MaxInt64
	}
	return limit * aggregateInputMultiplier
}

// inputBudget tracks the bytes a builtin has buffered across all of its file
// operands.
type inputBudget struct {
	limit int64
	used  int64
}

func newInputBudget(inv *Invocation) *inputBudget {
	return &inputBudget{limit: maxAggregateInputBytes(inv)}
}

// add charges n bytes to the budget and fails once the total exceeds it.
func (b *inputBudget) add(n int) error {
	if b == nil || b.limit == 0 {
		return nil
	}
	b.used += int64(n)
	if b.used > b.limit {
		return &ExitError{Code: 1, Err: commands.Diagnosticf("combined input exceeds maximum size of %d bytes", b.limit)}
	}
	return nil
}
