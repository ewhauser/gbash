package builtins

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestBoundedLineReaderStopsAtLimit(t *testing.T) {
	t.Parallel()

	reader := &boundedLineReader{r: newTestBufioReader(strings.Repeat("x", 10000) + "\nshort\n"), limit: 64}
	if _, err := reader.ReadLine('\n'); err == nil {
		t.Fatalf("ReadLine() error = nil, want input size error")
	} else if got := err.Error(); !strings.Contains(got, "input exceeds maximum file size of 64 bytes") {
		t.Fatalf("ReadLine() error = %q", got)
	}
}

func TestBoundedLineReaderMatchesReadBytes(t *testing.T) {
	t.Parallel()

	// Lines longer than bufio's default buffer must still be joined.
	long := strings.Repeat("y", 10000)
	reader := &boundedLineReader{r: newTestBufioReader("a\n" + long + "\ntail"), limit: 1 << 20}
	for _, want := range []string{"a\n", long + "\n"} {
		line, err := reader.ReadLine('\n')
		if err != nil || string(line) != want {
			t.Fatalf("ReadLine() = %d bytes, %v; want %d bytes", len(line), err, len(want))
		}
	}
	line, err := reader.ReadLine('\n')
	if string(line) != "tail" || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadLine() = %q, %v; want tail, EOF", line, err)
	}
}

func TestInputBudgetFailsPastLimit(t *testing.T) {
	t.Parallel()

	budget := &inputBudget{limit: 10}
	if err := budget.add(6); err != nil {
		t.Fatalf("add(6) error = %v", err)
	}
	err := budget.add(5)
	if err == nil || !strings.Contains(err.Error(), "combined input exceeds maximum size of 10 bytes") {
		t.Fatalf("add(5) error = %v, want combined input error", err)
	}
	if err := (&inputBudget{}).add(1 << 40); err != nil {
		t.Fatalf("unlimited budget add() error = %v", err)
	}
}

func newTestBufioReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}
