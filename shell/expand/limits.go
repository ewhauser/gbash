package expand

import (
	"fmt"
	"io"
	"strings"
)

// VariableSizeLimitError reports that an expansion would build a value larger
// than [Config.MaxVariableBytes].
type VariableSizeLimitError struct {
	// What names the expansion that produced the value, such as
	// "command substitution".
	What  string
	Limit int64
}

func (e VariableSizeLimitError) Error() string {
	return fmt.Sprintf("%s: value too large (limit %d bytes)", e.What, e.Limit)
}

// ErrVariableSizeLimitWrite is returned to writers that overflow a capped
// command substitution. It wraps [io.ErrClosedPipe] so commands stop quietly
// as they would on a closed pipe; the expansion reports the limit itself.
var ErrVariableSizeLimitWrite = fmt.Errorf("value size limit reached: %w", io.ErrClosedPipe)

// limitedStringWriter appends to b until the next write would grow it past
// limit, then fails every later write.
type limitedStringWriter struct {
	b        *strings.Builder
	limit    int64
	exceeded bool
}

func (w *limitedStringWriter) Write(p []byte) (int, error) {
	if w.exceeded || int64(w.b.Len())+int64(len(p)) > w.limit {
		w.exceeded = true
		return 0, ErrVariableSizeLimitWrite
	}
	return w.b.Write(p)
}

func (w *limitedStringWriter) WriteString(s string) (int, error) {
	if w.exceeded || int64(w.b.Len())+int64(len(s)) > w.limit {
		w.exceeded = true
		return 0, ErrVariableSizeLimitWrite
	}
	return w.b.WriteString(s)
}
