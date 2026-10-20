package interp

import (
	"context"
	"sync"
	"sync/atomic"
)

// maxLiveBackgroundJobs caps the background jobs a shell family runs at once,
// so `f() { f & f; }; f` cannot pile up goroutines faster than the command
// budget stops it.
const maxLiveBackgroundJobs = 256

// jobGroup tracks every background job and process substitution started by a
// runner and the subshells it spawns, so they can all be stopped and joined
// before the top-level run returns.
type jobGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	live   atomic.Int64
}

func newJobGroup() *jobGroup {
	ctx, cancel := context.WithCancel(context.Background())
	return &jobGroup{ctx: ctx, cancel: cancel}
}

// startBackground reserves a slot for a background job. It returns false
// when the live job cap is reached. On success, the returned context is ctx
// canceled when the group stops, and done must be called once the job ends.
func (g *jobGroup) startBackground(ctx context.Context) (context.Context, func(), bool) {
	if g == nil {
		return ctx, func() {}, true
	}
	if g.live.Add(1) > maxLiveBackgroundJobs {
		g.live.Add(-1)
		return nil, nil, false
	}
	g.wg.Add(1)
	jctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(g.ctx, cancel)
	return jctx, func() {
		stop()
		cancel()
		g.live.Add(-1)
		g.wg.Done()
	}, true
}

// startProcSubst registers a process substitution. Process substitutions are
// joined but not canceled when the group stops: they end once the shell side
// of their pipe is closed, which lets `exec > >(cat)` flush its output.
func (g *jobGroup) startProcSubst() func() {
	if g == nil {
		return func() {}
	}
	g.wg.Add(1)
	return g.wg.Done
}

// StopJobs cancels this shell family's background jobs, calls release (which
// should close the shell side of any process substitution pipes), and waits
// for every background job and process substitution to finish. Call it once
// the top-level run is over and before tearing down the resources jobs use.
func (r *Runner) StopJobs(release func()) {
	if r == nil || r.jobs == nil {
		if release != nil {
			release()
		}
		return
	}
	r.jobs.cancel()
	if release != nil {
		release()
	}
	r.jobs.wg.Wait()
}
