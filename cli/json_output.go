package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/trace"
)

type jsonExecutionResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	// StdoutBase64 and StderrBase64 carry the exact bytes, base64-encoded,
	// only when the stream is not valid UTF-8; Stdout and Stderr then hold a
	// lossy copy with invalid bytes replaced by U+FFFD.
	StdoutBase64    string            `json:"stdoutBase64,omitempty"`
	StderrBase64    string            `json:"stderrBase64,omitempty"`
	ExitCode        int               `json:"exitCode"`
	StdoutTruncated bool              `json:"stdoutTruncated"`
	StderrTruncated bool              `json:"stderrTruncated"`
	Timing          *jsonTiming       `json:"timing,omitempty"`
	Trace           *jsonTraceSummary `json:"trace,omitempty"`
}

type jsonTiming struct {
	StartedAt  string  `json:"startedAt"`
	FinishedAt string  `json:"finishedAt"`
	DurationMs float64 `json:"durationMs"`
}

type jsonTraceSummary struct {
	Schema      string `json:"schema"`
	SessionID   string `json:"sessionId,omitempty"`
	ExecutionID string `json:"executionId,omitempty"`
	EventCount  int    `json:"eventCount"`
}

func writeJSONExecutionResult(w io.Writer, payload *jsonExecutionResult) error {
	encoder := json.NewEncoder(writerOrDiscard(w))
	encoder.SetEscapeHTML(false)
	return encoder.Encode(payload)
}

func buildJSONExecutionResult(exitCode int, result *gbash.ExecutionResult, errText string) *jsonExecutionResult {
	payload := &jsonExecutionResult{
		ExitCode: exitCode,
	}
	if result != nil {
		payload.Stdout = result.Stdout
		payload.Stderr = result.Stderr
		payload.StdoutTruncated = result.StdoutTruncated
		payload.StderrTruncated = result.StderrTruncated
		payload.Timing = jsonTimingFromExecution(result)
		payload.Trace = jsonTraceFromEvents(result.Events)
	}
	payload.Stderr = appendStderrMessage(payload.Stderr, errText)
	payload.StdoutBase64 = base64IfNotUTF8(payload.Stdout)
	payload.StderrBase64 = base64IfNotUTF8(payload.Stderr)
	return payload
}

// base64IfNotUTF8 returns the base64 encoding of text when JSON encoding
// would otherwise replace invalid UTF-8 bytes, and "" when text is valid.
func base64IfNotUTF8(text string) string {
	if utf8.ValidString(text) {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(text))
}

func jsonTimingFromExecution(result *gbash.ExecutionResult) *jsonTiming {
	if result == nil || (result.StartedAt.IsZero() && result.FinishedAt.IsZero() && result.Duration == 0) {
		return nil
	}
	return &jsonTiming{
		StartedAt:  formatJSONTime(result.StartedAt),
		FinishedAt: formatJSONTime(result.FinishedAt),
		DurationMs: durationMilliseconds(result.Duration),
	}
}

func jsonTraceFromEvents(events []trace.Event) *jsonTraceSummary {
	if len(events) == 0 {
		return nil
	}
	first := events[0]
	schema := strings.TrimSpace(first.Schema)
	if schema == "" {
		schema = trace.SchemaVersion
	}
	return &jsonTraceSummary{
		Schema:      schema,
		SessionID:   first.SessionID,
		ExecutionID: first.ExecutionID,
		EventCount:  len(events),
	}
}

func formatJSONTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339Nano)
}

func durationMilliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func appendStderrMessage(stderrText, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return stderrText
	}

	var builder strings.Builder
	if stderrText != "" {
		builder.WriteString(stderrText)
		if !strings.HasSuffix(stderrText, "\n") {
			builder.WriteByte('\n')
		}
	}
	builder.WriteString(message)
	if !strings.HasSuffix(message, "\n") {
		builder.WriteByte('\n')
	}
	return builder.String()
}

func formatCLIError(name string, err error) string {
	if err == nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s: %v", name, err)
}

func writeCLIJSONError(stdout io.Writer, name string, exitCode int, err error) (int, error) {
	if jsonErr := writeJSONExecutionResult(stdout, buildJSONExecutionResult(exitCode, nil, formatCLIError(name, err))); jsonErr != nil {
		return 1, jsonErr
	}
	return exitCode, nil
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
