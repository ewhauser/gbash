package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunDemoInjectsOAuthWithoutLeakingSecret(t *testing.T) {
	report, err := runDemo(context.Background())
	if err != nil {
		t.Fatalf("runDemo() error = %v", err)
	}

	if got, want := len(report.Scenarios), 2; got != want {
		t.Fatalf("scenario count = %d, want %d", got, want)
	}
	if got, want := report.ScriptPath, demoScriptPath; got != want {
		t.Fatalf("ScriptPath = %q, want %q", got, want)
	}
	if !strings.Contains(report.ScriptSource, "curl -fsS \\") {
		t.Fatalf("ScriptSource = %q, want embedded shell script", report.ScriptSource)
	}

	baseline := report.Scenarios[0]
	if got, want := baseline.Name, baselineScenarioName; got != want {
		t.Fatalf("baseline name = %q, want %q", got, want)
	}
	if !baseline.Response.OK {
		t.Fatalf("baseline Response.OK = false, want true")
	}
	if got, want := baseline.Response.AuthenticatedAs, "demo-service-account"; got != want {
		t.Fatalf("baseline AuthenticatedAs = %q, want %q", got, want)
	}
	if got, want := baseline.Response.RequestID, requestID; got != want {
		t.Fatalf("baseline RequestID = %q, want %q", got, want)
	}
	if !baseline.Audit.AuthorizationInjected {
		t.Fatal("baseline AuthorizationInjected = false, want true")
	}
	if baseline.Audit.SandboxAuthorizationIn {
		t.Fatal("baseline SandboxAuthorizationIn = true, want false")
	}
	if baseline.Audit.AuthorizationOverrideApplied {
		t.Fatal("baseline AuthorizationOverrideApplied = true, want false")
	}
	if got, want := baseline.Audit.AuthorizationSource, tokenRef; got != want {
		t.Fatalf("baseline AuthorizationSource = %q, want %q", got, want)
	}
	if !baseline.ServerRequest.AuthorizationPresent {
		t.Fatal("baseline AuthorizationPresent = false, want true")
	}
	if !baseline.ServerRequest.AuthorizationValid {
		t.Fatal("baseline AuthorizationValid = false, want true")
	}
	if got, want := baseline.ServerRequest.RequestID, requestID; got != want {
		t.Fatalf("baseline server request id = %q, want %q", got, want)
	}
	if baseline.SecretVisibleInStdout {
		t.Fatal("baseline SecretVisibleInStdout = true, want false")
	}
	if baseline.SecretVisibleInTrace {
		t.Fatal("baseline SecretVisibleInTrace = true, want false")
	}
	assertTraceArgv(t, baseline.TraceArgv, []string{"curl", "-fsS", "-H", "X-Request-ID: " + requestID, demoRequestURL})

	override := report.Scenarios[1]
	if got, want := override.Name, overrideScenarioName; got != want {
		t.Fatalf("override name = %q, want %q", got, want)
	}
	if !override.Response.OK {
		t.Fatalf("override Response.OK = false, want true")
	}
	if got, want := override.Response.AuthenticatedAs, "demo-service-account"; got != want {
		t.Fatalf("override AuthenticatedAs = %q, want %q", got, want)
	}
	if got, want := override.Response.RequestID, overrideAttemptID; got != want {
		t.Fatalf("override RequestID = %q, want %q", got, want)
	}
	if !override.Audit.AuthorizationInjected {
		t.Fatal("override AuthorizationInjected = false, want true")
	}
	if !override.Audit.SandboxAuthorizationIn {
		t.Fatal("override SandboxAuthorizationIn = false, want true")
	}
	if !override.Audit.AuthorizationOverrideApplied {
		t.Fatal("override AuthorizationOverrideApplied = false, want true")
	}
	if got, want := override.Audit.AuthorizationSource, tokenRef; got != want {
		t.Fatalf("override AuthorizationSource = %q, want %q", got, want)
	}
	if !override.ServerRequest.AuthorizationPresent {
		t.Fatal("override AuthorizationPresent = false, want true")
	}
	if !override.ServerRequest.AuthorizationValid {
		t.Fatal("override AuthorizationValid = false, want true")
	}
	if got, want := override.ServerRequest.RequestID, overrideAttemptID; got != want {
		t.Fatalf("override server request id = %q, want %q", got, want)
	}
	if override.SecretVisibleInStdout {
		t.Fatal("override SecretVisibleInStdout = true, want false")
	}
	if override.SecretVisibleInTrace {
		t.Fatal("override SecretVisibleInTrace = true, want false")
	}
	assertTraceArgv(t, override.TraceArgv, []string{"curl", "-fsS", "-H", "Authorization: " + sandboxForgedAuth, "-H", "X-Request-ID: " + overrideAttemptID, demoRequestURL})
}

func assertTraceArgv(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("TraceArgv length = %d, want %d (%q)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TraceArgv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
