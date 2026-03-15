package fuzztest

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/network"
	"github.com/ewhauser/gbash/policy"
)

const (
	DefaultMaxScriptBytes    = 4 << 10
	DefaultExecutionTimeout  = 500 * time.Millisecond
	DefaultWarmupTimeout     = 5 * time.Second
	defaultMaxAcceptableTime = 2 * time.Second
)

type RuntimeOptions struct {
	Registry      commands.CommandRegistry
	NetworkClient network.Client
	Policy        policy.Policy
}

type assertOptions struct {
	allowExecutionTimeout   bool
	requireExecutionTimeout bool
}

func NewRuntime(tb testing.TB, opts RuntimeOptions) *gbash.Runtime {
	tb.Helper()

	pol := opts.Policy
	if pol == nil {
		pol = policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits: policy.Limits{
				MaxCommandCount:      200,
				MaxGlobOperations:    2000,
				MaxLoopIterations:    200,
				MaxSubstitutionDepth: 8,
				MaxStdoutBytes:       16 << 10,
				MaxStderrBytes:       16 << 10,
				MaxFileBytes:         128 << 10,
			},
		})
	}

	rt, err := gbash.New(gbash.WithConfig(&gbash.Config{
		Registry:      opts.Registry,
		NetworkClient: opts.NetworkClient,
		Policy:        pol,
	}))
	if err != nil {
		tb.Fatalf("gbash.New() error = %v", err)
	}
	return rt
}

func NewSession(tb testing.TB, rt *gbash.Runtime) *gbash.Session {
	tb.Helper()

	session, err := rt.NewSession(context.Background())
	if err != nil {
		tb.Fatalf("Runtime.NewSession() error = %v", err)
	}
	return session
}

func RunSessionScript(t *testing.T, session *gbash.Session, name string, script []byte) (*gbash.ExecutionResult, error) {
	t.Helper()
	return RunSessionScriptWithTimeout(t, session, name, script, DefaultExecutionTimeout)
}

func RunSessionScriptWithTimeout(t *testing.T, session *gbash.Session, name string, script []byte, timeout time.Duration) (*gbash.ExecutionResult, error) {
	t.Helper()

	if len(script) > DefaultMaxScriptBytes {
		t.Skip()
	}

	return session.Exec(context.Background(), &gbash.ExecutionRequest{
		Name:    name,
		Script:  string(script),
		Timeout: timeout,
	})
}

func AssertSuccessfulFuzzExecution(t *testing.T, script []byte, result *gbash.ExecutionResult, err error) {
	t.Helper()

	AssertSecureFuzzOutcome(t, script, result, err)
	if result == nil {
		t.Fatalf("nil result for script %q", previewScript(script))
		return
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d\nscript=%q\nstderr=%q", result.ExitCode, previewScript(script), result.Stderr)
	}
}

func AssertSecureFuzzOutcome(t *testing.T, script []byte, result *gbash.ExecutionResult, err error) {
	t.Helper()
	assertSecureFuzzOutcomeWithOptions(t, script, result, err, assertOptions{})
}

func AssertSecureFuzzOutcomeAllowExecutionTimeout(t *testing.T, script []byte, result *gbash.ExecutionResult, err error) {
	t.Helper()
	assertSecureFuzzOutcomeWithOptions(t, script, result, err, assertOptions{allowExecutionTimeout: true})
}

func AssertExecutionTimeoutFuzzOutcome(t *testing.T, script []byte, result *gbash.ExecutionResult, err error) {
	t.Helper()
	assertSecureFuzzOutcomeWithOptions(t, script, result, err, assertOptions{
		allowExecutionTimeout:   true,
		requireExecutionTimeout: true,
	})
}

func assertSecureFuzzOutcomeWithOptions(t *testing.T, script []byte, result *gbash.ExecutionResult, err error, opts assertOptions) {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error %T: %v\nscript=%q", err, err, previewScript(script))
	}
	assertNoExecutionTimeout(t, script, result, opts)
	assertNoHostPathLeak(t, script, result, err)
	assertNoSensitiveDisclosure(t, script, result, err)
	assertNoInternalCrashOutput(t, script, result, err)
	assertNoRunawayExecution(t, script, result)
}

func assertNoExecutionTimeout(t *testing.T, script []byte, result *gbash.ExecutionResult, opts assertOptions) {
	t.Helper()

	timedOut := isNormalizedExecutionTimeout(result)
	if timedOut && !opts.allowExecutionTimeout {
		t.Fatalf("unexpected execution timeout\nscript=%q\nstderr=%q", previewScript(script), result.Stderr)
	}
	if !timedOut && opts.requireExecutionTimeout {
		if result == nil {
			t.Fatalf("expected normalized execution timeout\nscript=%q", previewScript(script))
		}
		t.Fatalf("expected normalized execution timeout\nscript=%q\nexit=%d\nstderr=%q", previewScript(script), result.ExitCode, result.Stderr)
	}
}

func isNormalizedExecutionTimeout(result *gbash.ExecutionResult) bool {
	if result == nil || result.ExitCode != 124 {
		return false
	}
	if strings.Contains(result.ControlStderr, "execution timed out") {
		return true
	}
	return strings.Contains(result.Stderr, "execution timed out")
}

func assertNoHostPathLeak(t *testing.T, script []byte, result *gbash.ExecutionResult, err error) {
	t.Helper()

	checks := []string{}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil && cwd != "" {
		checks = append(checks, cwd)
	}
	if home, homeErr := os.UserHomeDir(); homeErr == nil && home != "" {
		checks = append(checks, home)
	}

	var haystacks []string
	if result != nil {
		haystacks = append(haystacks, result.Stderr)
	}
	if err != nil {
		haystacks = append(haystacks, err.Error())
	}

	for _, token := range checks {
		if token == "" || bytes.Contains(script, []byte(token)) {
			continue
		}
		for _, haystack := range haystacks {
			if strings.Contains(haystack, token) {
				t.Fatalf("host path leak %q in fuzz outcome\nscript=%q\noutput=%q", token, previewScript(script), haystack)
			}
		}
	}
}

func assertNoSensitiveDisclosure(t *testing.T, script []byte, result *gbash.ExecutionResult, err error) {
	t.Helper()

	tokens := sensitiveTokens()
	var haystacks []string
	if result != nil {
		haystacks = append(haystacks, result.Stdout, result.Stderr)
	}
	if err != nil {
		haystacks = append(haystacks, err.Error())
	}

	for _, token := range tokens {
		if token == "" || len(token) < 4 || bytes.Contains(script, []byte(token)) {
			continue
		}
		for _, haystack := range haystacks {
			if strings.Contains(haystack, token) {
				t.Fatalf("sensitive host token leak %q in fuzz outcome\nscript=%q\noutput=%q", token, previewScript(script), haystack)
			}
		}
	}
}

func assertNoInternalCrashOutput(t *testing.T, script []byte, result *gbash.ExecutionResult, err error) {
	t.Helper()

	var haystacks []string
	if result != nil {
		haystacks = append(haystacks, result.Stderr)
	}
	if err != nil {
		haystacks = append(haystacks, err.Error())
	}

	for _, haystack := range haystacks {
		if containsInternalCrashOutput(haystack) {
			t.Fatalf("internal crash output detected\nscript=%q\noutput=%q", previewScript(script), haystack)
		}
	}
}

func assertNoRunawayExecution(t *testing.T, script []byte, result *gbash.ExecutionResult) {
	t.Helper()

	if result == nil {
		return
	}
	if result.Duration <= defaultMaxAcceptableTime {
		return
	}
	t.Fatalf("runaway execution duration %s exceeds %s\nscript=%q", result.Duration, defaultMaxAcceptableTime, previewScript(script))
}

func sensitiveTokens() []string {
	tokens := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		tokens = append(tokens, value)
	}

	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home)
	}
	add(os.Getenv("HOME"))
	add(os.Getenv("USER"))
	add(os.Getenv("LOGNAME"))
	add(os.Getenv("SHELL"))
	add(os.Getenv("TMPDIR"))
	if hostname, err := os.Hostname(); err == nil {
		add(hostname)
	}
	return tokens
}

func containsInternalCrashOutput(output string) bool {
	patterns := []string{
		"panic:",
		"fatal error:",
		"runtime error:",
		"SIGSEGV",
		"stack trace",
		"goroutine ",
	}
	for _, pattern := range patterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

func previewScript(script []byte) string {
	const maxPreviewBytes = 160
	if len(script) <= maxPreviewBytes {
		return string(script)
	}
	return string(script[:maxPreviewBytes]) + "...(truncated)"
}
