package interp

import (
	"context"
	"errors"
	"io"

	"github.com/ewhauser/gbash/shell/expand"
)

// errVariableSizeLimit stops builtins such as read and mapfile from
// buffering input past MaxVariableBytes.
var errVariableSizeLimit = errors.New("value too large")

// arrayElementOverheadBytes is charged per array element on top of its
// content, so arrays of many tiny elements are bounded as well.
const arrayElementOverheadBytes = 16

const variableLimitHint = ", increase policy.Limits.MaxVariableBytes"

const totalVariableLimitHint = ", increase policy.Limits.MaxTotalVariableBytes"

// variableValueBytes approximates the memory a variable's value holds.
func variableValueBytes(vr expand.Variable) int64 {
	switch vr.Kind {
	case expand.String, expand.NameRef:
		return int64(len(vr.Str))
	case expand.Indexed:
		size := int64(len(vr.List)) * arrayElementOverheadBytes
		for _, elem := range vr.List {
			size += int64(len(elem))
		}
		return size
	case expand.Associative:
		size := int64(len(vr.Map)) * arrayElementOverheadBytes
		for key, value := range vr.Map {
			size += int64(len(key)) + int64(len(value))
		}
		return size
	default:
		return 0
	}
}

// storedVariableBytes is the size a bound variable counts toward
// MaxTotalVariableBytes; unset bindings hold no value.
func storedVariableBytes(vr expand.Variable) int64 {
	if !vr.IsSet() {
		return 0
	}
	return variableValueBytes(vr)
}

// environValueBytes totals the values bound in every layer of env, counting a
// shadowed binding as well as the one that hides it, since both are held.
func environValueBytes(env expand.Environ) int64 {
	var total int64
	for env != nil {
		switch e := env.(type) {
		case *overlayEnviron:
			total += e.valueBytes
			env = e.parent
		case *shadowWriteEnviron:
			if e.shadowSet {
				total += storedVariableBytes(e.shadow)
			}
			env = e.parent
		default:
			return total
		}
	}
	return total
}

// totalVariablesTooLarge reports whether replacing prev with vr would take the
// shell's variables past MaxTotalVariableBytes.
func (r *Runner) totalVariablesTooLarge(vr, prev expand.Variable) bool {
	if r.maxTotalVariableBytes <= 0 || !vr.IsSet() {
		return false
	}
	growth := storedVariableBytes(vr) - storedVariableBytes(prev)
	if growth <= 0 {
		return false
	}
	return environValueBytes(r.writeEnv)+growth > r.maxTotalVariableBytes
}

// totalVariableLimitExceeded reports a write that would take the shell's
// variables past MaxTotalVariableBytes, and stops the shell like
// [Runner.variableLimitExceeded].
func (r *Runner) totalVariableLimitExceeded(name string) {
	if !r.variableLimitHit {
		r.errf("%s: total variable size too large (limit %d bytes)%s\n", name, r.maxTotalVariableBytes, totalVariableLimitHint)
	}
	r.variableLimitHit = true
	r.applyVariableLimitExit()
}

// wordListTooLarge reports whether expanded words, such as a command's
// arguments, exceed MaxVariableBytes when sized like an indexed array. This
// bounds argv and positional parameters, which "$@" "$@" can double.
func (r *Runner) wordListTooLarge(words []string) bool {
	if r.maxVariableBytes <= 0 {
		return false
	}
	size := int64(len(words)) * arrayElementOverheadBytes
	for _, word := range words {
		size += int64(len(word))
	}
	return size > r.maxVariableBytes
}

// variableTooLarge reports whether storing vr would exceed MaxVariableBytes.
func (r *Runner) variableTooLarge(vr expand.Variable) bool {
	if r.maxVariableBytes <= 0 || !vr.IsSet() {
		return false
	}
	return variableValueBytes(vr) > r.maxVariableBytes
}

// variableLimitExceeded reports a value for name that is over
// MaxVariableBytes and stops the current shell with status 126, as the other
// policy budgets do. Interactive shells abort only the current command.
func (r *Runner) variableLimitExceeded(name string) {
	if !r.variableLimitHit {
		r.errf("%v%s\n", expand.VariableSizeLimitError{What: name, Limit: r.maxVariableBytes}, variableLimitHint)
	}
	r.variableLimitHit = true
	r.applyVariableLimitExit()
}

func (r *Runner) applyVariableLimitExit() {
	r.applyLimitExit()
}

// reassertVariableLimitExit restores the limit status after a command whose
// builtin or assignment code overwrote r.exit once the limit was hit.
func (r *Runner) reassertVariableLimitExit() {
	if !r.variableLimitHit {
		return
	}
	r.variableLimitHit = false
	r.applyVariableLimitExit()
}

// cmdSubstLimitContext lets a command substitution stop as soon as its
// output overflows the expansion's MaxVariableBytes cap; commands that ignore
// the write error would otherwise keep running with their output discarded.
func (r *Runner) cmdSubstLimitContext(ctx context.Context, w io.Writer) (context.Context, io.Writer, context.CancelCauseFunc) {
	if r.maxVariableBytes <= 0 {
		return ctx, w, func(error) {}
	}
	ctx, cancel := context.WithCancelCause(ctx)
	return ctx, &cancelOnLimitWriter{w: w, cancel: cancel}, cancel
}

// cancelOnLimitWriter must be used by pointer: file-descriptor code compares
// writers with ==, which panics on a struct holding a func.
type cancelOnLimitWriter struct {
	w      io.Writer
	cancel context.CancelCauseFunc
}

func (w *cancelOnLimitWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if errors.Is(err, expand.ErrVariableSizeLimitWrite) {
		w.cancel(err)
	}
	return n, err
}
