package commands

import (
	"io"
	"time"

	"github.com/ewhauser/gbash/trace"
)

// ExecutionRequest describes a non-interactive nested shell or command
// execution launched through [Invocation.Exec].
type ExecutionRequest struct {
	Name            string
	Interpreter     string
	ShellVariant    ShellVariant
	PassthroughArgs []string
	ScriptPath      string
	Script          string
	// Command runs an already-tokenized command argv without shell parsing.
	// Script and Command are mutually exclusive.
	Command []string
	// CommandPath optionally overrides the executable looked up for Command[0]
	// while preserving Command[0] as the presented argv0.
	CommandPath string
	// CommandName optionally overrides the resolved command name used for
	// policy checks and tracing when CommandPath is set.
	CommandName    string
	Args           []string
	StartupOptions []string
	Env            map[string]string
	WorkDir        string
	Timeout        time.Duration
	ReplaceEnv     bool
	Interactive    bool
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	// SpillDir optionally enables large-output spilling. When non-empty,
	// stdout/stderr overflow beyond the policy byte limits is written to temp
	// files in this directory (see [ExecutionResult.StdoutSpillPath]). Empty
	// keeps the legacy truncate-and-drop behavior.
	SpillDir string
}

// ExecutionResult reports the outcome of an [ExecutionRequest].
type ExecutionResult struct {
	ExitCode      int
	ShellExited   bool
	Stdout        string
	Stderr        string
	ControlStderr string
	// CommandNotFound reports that nested command resolution failed before any
	// child process or builtin was invoked.
	CommandNotFound bool
	FinalEnv        map[string]string
	StartedAt       time.Time
	FinishedAt      time.Time
	Duration        time.Duration
	// Events contains structured execution events when tracing is enabled on the
	// parent runtime. It is empty by default.
	Events          []trace.Event
	StdoutTruncated bool
	StderrTruncated bool
	// TimedOut reports that execution was stopped by the request timeout.
	TimedOut bool
	// StdoutSpillPath / StderrSpillPath point at temp files holding the
	// overflow when the corresponding stream exceeded its byte limit and
	// spilling was enabled. Empty when nothing was spilled.
	StdoutSpillPath string
	StderrSpillPath string
}

// InteractiveRequest describes an interactive nested shell launched through
// [Invocation.Interact].
type InteractiveRequest struct {
	Name           string
	ShellVariant   ShellVariant
	Args           []string
	StartupOptions []string
	Env            map[string]string
	WorkDir        string
	ReplaceEnv     bool
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
}

// InteractiveResult reports the outcome of an [InteractiveRequest].
type InteractiveResult struct {
	ExitCode int
}
