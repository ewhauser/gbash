package interp

import (
	"fmt"

	"github.com/ewhauser/gbash/shell/syntax"
)

// SubshellDepthLimitError reports a command substitution, subshell or process
// substitution nested deeper than MaxSubshellDepth allows.
type SubshellDepthLimitError struct {
	Limit int64
}

func (e SubshellDepthLimitError) Error() string {
	return fmt.Sprintf("subshell nesting limit exceeded (%d), increase policy.Limits.MaxSubstitutionDepth", e.Limit)
}

// GlobLimitError reports that pathname expansion used up the glob
// operation budget.
type GlobLimitError struct {
	Limit int64
}

func (e GlobLimitError) Error() string {
	return fmt.Sprintf("Glob operation limit exceeded (%d), increase policy.Limits.MaxGlobOperations", e.Limit)
}

// subshellDepthExceeded reports whether starting a command substitution,
// subshell or process substitution from r would nest too deeply.
func (r *Runner) subshellDepthExceeded() bool {
	return r.maxSubshellDepth > 0 && r.subshellDepth >= r.maxSubshellDepth
}

// subshellDepthLimitExceeded reports a subshell nested too deeply and stops
// the current shell with status 126, like the other policy budgets.
func (r *Runner) subshellDepthLimitExceeded() {
	r.errf("%v\n", SubshellDepthLimitError{Limit: r.maxSubshellDepth})
	r.applyLimitExit()
}

// functionDepthLimitExceeded reports a function call nested deeper than
// MaxFunctionDepth and stops the current shell with status 126. Bash prints
// the same message when FUNCNEST is exceeded.
func (r *Runner) functionDepthLimitExceeded(name string) {
	r.errf("%s: maximum function nesting level exceeded (%d), increase policy.Limits.MaxFunctionDepth\n", name, r.maxFunctionDepth)
	r.applyLimitExit()
}

// applyLimitExit stops the current shell with status 126 after a sandbox
// resource limit was hit. Interactive shells abort only the current command.
func (r *Runner) applyLimitExit() {
	r.exit.code = 126
	if r.interactive {
		r.commandAborted = true
	} else {
		r.exit.exiting = true
	}
}

// isSyntheticPipelineSubshell reports whether cm is the subshell the shell
// core wraps around a pipeline stage. Those reuse the inner statement's span
// exactly, while user-written subshells include their parentheses.
func isSyntheticPipelineSubshell(cm *syntax.Subshell) bool {
	if cm == nil || len(cm.Stmts) != 1 || cm.Stmts[0] == nil {
		return false
	}
	inner := cm.Stmts[0]
	return cm.Lparen == inner.Pos() && cm.Rparen == inner.End()
}
