package gojq

import "strconv"

// MaxDepth bounds how deeply gojq's recursive helpers (comparison, encoding,
// contains, flatten, object merging and path updates) descend into nested
// values. jq programs can build values nested arbitrarily deep without
// recursing in Go, so an unbounded helper would overflow the goroutine stack,
// which is a fatal error that terminates the whole process.
const MaxDepth = 10000

// DepthLimitError reports an operation on a value nested beyond [MaxDepth].
//
// Helpers that cannot return an error panic with a *DepthLimitError. The
// iterator returned by [Code.Run] converts that panic into its final value and
// stops, so the error cannot be caught with try.
type DepthLimitError struct {
	Op string
}

func (err *DepthLimitError) Error() string {
	return err.Op + ": value nesting exceeds depth limit of " + strconv.Itoa(MaxDepth)
}

func checkDepth(op string, depth int) {
	if depth > MaxDepth {
		panic(&DepthLimitError{Op: op})
	}
}
