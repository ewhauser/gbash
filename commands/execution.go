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
	// files in this host directory (see [ExecutionResult.StdoutSpillPath]).
	// The caller owns those files and must remove them. If a spill file cannot
	// be created or written, it is discarded and the stream falls back to
	// truncation. Empty keeps the legacy truncate-and-drop behavior.
	SpillDir string
	// MaxSpillBytes caps each spill file. Overflow past the cap is dropped and
	// reported via [ExecutionResult.StdoutSpillTruncated].
	// Zero or negative uses a 64 MiB default.
	MaxSpillBytes int64
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
	// spilling was enabled. The file holds only the bytes past the limit:
	// Stdout followed by the file contents is the full stream, up to
	// MaxSpillBytes of overflow. Empty when nothing was spilled.
	StdoutSpillPath string
	StderrSpillPath string
	// StdoutSpillTruncated / StderrSpillTruncated report that the overflow
	// exceeded MaxSpillBytes, so the spill file was cut short and the bytes
	// past the cap were dropped.
	StdoutSpillTruncated bool
	StderrSpillTruncated bool
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
