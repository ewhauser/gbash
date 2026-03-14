package runtime

import (
	"context"
	"time"

	"github.com/ewhauser/gbash/trace"
)

type TraceMode uint8

const (
	TraceOff TraceMode = iota
	TraceRedacted
	TraceRaw
)

type TraceConfig struct {
	Mode    TraceMode
	OnEvent func(context.Context, trace.Event)
}

type LogKind string

const (
	LogExecStart  LogKind = "exec.start"
	LogStdout     LogKind = "stdout"
	LogStderr     LogKind = "stderr"
	LogExecFinish LogKind = "exec.finish"
	LogExecError  LogKind = "exec.error"
)

type LogEvent struct {
	Kind        LogKind
	SessionID   string
	ExecutionID string
	Name        string
	WorkDir     string
	ExitCode    int
	Duration    time.Duration
	Output      string
	Truncated   bool
	ShellExited bool
	Error       string
}

type LogCallback func(context.Context, LogEvent)
