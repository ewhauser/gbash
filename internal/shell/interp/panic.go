package interp

import (
	"fmt"
	"strings"
)

// SanitizeRunnerPanic maps a value recovered from a runner panic to a
// user-facing message that does not leak Go internals.
func SanitizeRunnerPanic(recovered any) string {
	message := fmt.Sprint(recovered)
	switch {
	case strings.HasPrefix(message, "unhandled >& arg:"),
		strings.HasPrefix(message, "unhandled > arg:"),
		strings.HasPrefix(message, "unhandled < arg:"),
		strings.HasPrefix(message, "unsupported redirect fd:"),
		strings.HasPrefix(message, "unhandled redirect op:"):
		return "invalid redirection"
	default:
		return "shell execution failed"
	}
}

// recoverAsyncPanic must be deferred directly by goroutines that run a
// subshell (pipeline left-hand sides, background jobs and process
// substitutions). The foreground shell's panics are recovered by the caller of
// [Runner.Run], but nothing above these goroutines would catch a panic, so one
// misbehaving command would crash the whole host process. Instead the
// subshell reports the sanitized message and exits with status 2, matching the
// foreground recovery.
func (r *Runner) recoverAsyncPanic() {
	recovered := recover()
	if recovered == nil {
		return
	}
	r.errf("%s\n", SanitizeRunnerPanic(recovered))
	r.exit = exitStatus{code: 2}
}
