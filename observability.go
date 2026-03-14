package gbash

import internalruntime "github.com/ewhauser/gbash/internal/runtime"

// TraceMode controls whether gbash records structured execution events.
type TraceMode = internalruntime.TraceMode

const (
	// TraceOff disables structured execution events.
	TraceOff = internalruntime.TraceOff
	// TraceRedacted enables structured events with argv redaction for
	// secret-bearing values. This is the recommended mode for agent workloads.
	TraceRedacted = internalruntime.TraceRedacted
	// TraceRaw enables full structured events without argv redaction.
	//
	// This mode is unsafe for shared logs or centralized telemetry unless the
	// embedder controls retention and sink access tightly.
	TraceRaw = internalruntime.TraceRaw
)

// TraceConfig configures structured execution tracing.
//
// When Mode is [TraceOff], [ExecutionResult.Events] is empty and OnEvent is not
// called. Interactive executions only deliver events to OnEvent; they do not
// return events in an [InteractiveResult].
type TraceConfig = internalruntime.TraceConfig

// LogKind identifies a high-level execution lifecycle log event.
type LogKind = internalruntime.LogKind

const (
	// LogExecStart fires before the shell engine begins execution.
	LogExecStart = internalruntime.LogExecStart
	// LogStdout carries the final captured stdout for one execution.
	LogStdout = internalruntime.LogStdout
	// LogStderr carries the final captured stderr for one execution.
	LogStderr = internalruntime.LogStderr
	// LogExecFinish fires when execution completes normally, including shell
	// exit statuses and timeout/cancel control outcomes.
	LogExecFinish = internalruntime.LogExecFinish
	// LogExecError fires when gbash returns an unexpected runtime error rather
	// than a normal shell exit status.
	LogExecError = internalruntime.LogExecError
)

// LogEvent describes one top-level execution lifecycle log callback.
type LogEvent = internalruntime.LogEvent

// LogCallback receives top-level execution lifecycle logs.
//
// Callbacks run synchronously on the execution path. Panics are recovered and
// ignored so observability hooks do not fail shell execution.
type LogCallback = internalruntime.LogCallback
