package awk

import (
	"context"
	"errors"
	"sync"
	"time"
)

// awkDefaultMaxDuration bounds a program's run time when the request has no
// deadline. goawk never consults the shell's loop or command budgets, so a
// program such as `BEGIN { while (1); }` would otherwise run forever.
const awkDefaultMaxDuration = 60 * time.Second

var errAWKTimeLimit = errors.New("awk: execution exceeded the time limit")

// awkTimeBudget is a run-time limit that stops counting while awk waits on
// something outside its control, such as reading stdin or a system() command,
// so a slow upstream producer cannot exhaust it.
type awkTimeBudget struct {
	mu        sync.Mutex
	timer     *time.Timer
	cancel    context.CancelCauseFunc
	remaining time.Duration
	started   time.Time
	paused    int
}

// newAWKTimeBudget returns ctx unchanged with an inert budget when ctx
// already has a deadline; otherwise the returned context is canceled with
// errAWKTimeLimit once maxDuration of unpaused time has elapsed.
func newAWKTimeBudget(ctx context.Context, maxDuration time.Duration) (context.Context, *awkTimeBudget) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, &awkTimeBudget{}
	}
	if maxDuration <= 0 {
		maxDuration = awkDefaultMaxDuration
	}
	budgetCtx, cancel := context.WithCancelCause(ctx)
	b := &awkTimeBudget{cancel: cancel, remaining: maxDuration, started: time.Now()}
	b.timer = time.AfterFunc(maxDuration, func() { cancel(errAWKTimeLimit) })
	return budgetCtx, b
}

func (b *awkTimeBudget) pause() {
	if b.timer == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.paused == 0 && b.timer.Stop() {
		b.remaining -= time.Since(b.started)
	}
	b.paused++
}

func (b *awkTimeBudget) resume() {
	if b.timer == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.paused--
	if b.paused == 0 {
		b.started = time.Now()
		b.timer.Reset(max(b.remaining, 0))
	}
}

func (b *awkTimeBudget) stop() {
	if b.timer == nil {
		return
	}
	b.timer.Stop()
	b.cancel(context.Canceled)
}

// exceeded reports whether the budget, rather than the request, stopped ctx.
func (b *awkTimeBudget) exceeded(ctx context.Context) bool {
	return b.timer != nil && errors.Is(context.Cause(ctx), errAWKTimeLimit)
}
