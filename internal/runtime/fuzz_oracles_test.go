package runtime

import (
	"os"
	"strings"
	"testing"
	"time"
)

var internalLeakPatterns = []string{
	"panic:",
	"runtime error:",
	"fatal error:",
	"goroutine ",
}

var sensitiveLeakPatterns = []string{
	"root:x:",
	"daemon:x:",
}

const fuzzMaxAcceptableDuration = 2 * time.Second

func assertSecureFuzzOutcome(t *testing.T, script []byte, result *ExecutionResult, err error) {
	t.Helper()

	assertSecureFuzzOutcomeWithOptions(t, script, result, err, fuzzOutcomeOptions{})
}

func assertSecureFuzzOutcomeAllowExecutionTimeout(t *testing.T, script []byte, result *ExecutionResult, err error) {
	t.Helper()

	assertSecureFuzzOutcomeWithOptions(t, script, result, err, fuzzOutcomeOptions{
		allowExecutionTimeout: true,
	})
}

func assertExecutionTimeoutFuzzOutcome(t *testing.T, script []byte, result *ExecutionResult, err error) {
	t.Helper()

	assertSecureFuzzOutcomeWithOptions(t, script, result, err, fuzzOutcomeOptions{
		allowExecutionTimeout:   true,
		requireExecutionTimeout: true,
	})
}

type fuzzOutcomeOptions struct {
	allowExecutionTimeout   bool
	requireExecutionTimeout bool
}

func assertSecureFuzzOutcomeWithOptions(t *testing.T, script []byte, result *ExecutionResult, err error, opts fuzzOutcomeOptions) {
	t.Helper()

	assertBaseFuzzOutcome(t, script, result, err)
	assertNoInternalLeak(t, script, result, err)
	assertNoSensitiveLeak(t, script, result, err)
	assertExecutionTimeoutState(t, script, result, opts)
	assertNoRunawayExecution(t, script, result)
}

func assertExecutionTimeoutState(t *testing.T, script []byte, result *ExecutionResult, opts fuzzOutcomeOptions) {
	t.Helper()

	timedOut := isNormalizedExecutionTimeout(result)
	if timedOut && !opts.allowExecutionTimeout {
		t.Fatalf("unexpected execution timeout\nscript=%q\nstderr=%q", previewFuzzScript(script), result.Stderr)
	}
	if !timedOut && opts.requireExecutionTimeout {
		if result == nil {
			t.Fatalf("expected normalized execution timeout\nscript=%q", previewFuzzScript(script))
		}
		t.Fatalf("expected normalized execution timeout\nscript=%q\nexit=%d\nstderr=%q", previewFuzzScript(script), result.ExitCode, result.Stderr)
	}
}

func isNormalizedExecutionTimeout(result *ExecutionResult) bool {
	if result == nil || result.ExitCode != 124 {
		return false
	}
	if strings.Contains(result.ControlStderr, "execution timed out") {
		return true
	}
	return strings.Contains(result.Stderr, "execution timed out")
}

func assertNoInternalLeak(t *testing.T, script []byte, result *ExecutionResult, err error) {
	t.Helper()

	for _, haystack := range fuzzHaystacks(result, err) {
		for _, pattern := range internalLeakPatterns {
			if strings.Contains(haystack, pattern) {
				t.Fatalf("internal leak pattern %q in fuzz outcome\nscript=%q\noutput=%q", pattern, previewFuzzScript(script), haystack)
			}
		}
	}
}

func assertNoSensitiveLeak(t *testing.T, script []byte, result *ExecutionResult, err error) {
	t.Helper()

	for _, haystack := range fuzzHaystacks(result, err) {
		for _, pattern := range sensitiveLeakPatterns {
			if strings.Contains(haystack, pattern) {
				t.Fatalf("sensitive leak pattern %q in fuzz outcome\nscript=%q\noutput=%q", pattern, previewFuzzScript(script), haystack)
			}
		}
		if home, homeErr := os.UserHomeDir(); homeErr == nil && home != "" && !bytesContains(script, home) && strings.Contains(haystack, home) {
			t.Fatalf("host home leaked in fuzz outcome\nscript=%q\noutput=%q", previewFuzzScript(script), haystack)
		}
	}
}

func fuzzHaystacks(result *ExecutionResult, err error) []string {
	haystacks := make([]string, 0, 3)
	if result != nil {
		haystacks = append(haystacks, result.Stdout, result.Stderr)
	}
	if err != nil {
		haystacks = append(haystacks, err.Error())
	}
	return haystacks
}

func bytesContains(script []byte, token string) bool {
	return strings.Contains(string(script), token)
}

func assertNoRunawayExecution(t *testing.T, script []byte, result *ExecutionResult) {
	t.Helper()

	if result == nil {
		return
	}
	if result.Duration <= fuzzMaxAcceptableDuration {
		return
	}
	t.Fatalf("runaway execution duration %s exceeds %s\nscript=%q", result.Duration, fuzzMaxAcceptableDuration, previewFuzzScript(script))
}
