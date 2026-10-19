package jq

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// jqDefaultMaxDuration bounds one filter evaluation when the request has no
// deadline, so programs such as `repeat(.)` or `def f: f; f` stop even though
// they never touch the shell's loop or command budgets.
const jqDefaultMaxDuration = 60 * time.Second

var errJQTimeLimit = fmt.Errorf("evaluation exceeded the %s time limit", jqDefaultMaxDuration)

// jqEvalContext derives the context one filter evaluation runs under, adding
// the default duration when ctx has no deadline. gojq checks the context
// before every instruction, so the limit stops a runaway program promptly.
func jqEvalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return jqEvalContextWithLimit(ctx, jqDefaultMaxDuration)
}

func jqEvalContextWithLimit(ctx context.Context, maxDuration time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	evalCtx, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(maxDuration, func() { cancel(errJQTimeLimit) })
	return evalCtx, func() {
		timer.Stop()
		cancel(context.Canceled)
	}
}

// jqEvalLimitError reports errJQTimeLimit when the default duration stopped
// the evaluation, and nil otherwise.
func jqEvalLimitError(evalCtx context.Context) error {
	if cause := context.Cause(evalCtx); errors.Is(cause, errJQTimeLimit) {
		return cause
	}
	return nil
}
