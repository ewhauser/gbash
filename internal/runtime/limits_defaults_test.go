package runtime

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

func TestDefaultLimitsSetEveryField(t *testing.T) {
	t.Parallel()
	limits := reflect.ValueOf(defaultLimits())
	for i := range limits.NumField() {
		if limits.Field(i).IsZero() {
			t.Errorf("defaultLimits().%s is zero; every policy.Limits field needs a runtime default", limits.Type().Field(i).Name)
		}
	}
}

func TestCustomPolicyZeroLimitsInheritDefaults(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{
		Policy: policy.NewStatic(&policy.Config{ReadRoots: []string{"/"}, WriteRoots: []string{"/"}}),
	})
	if got, want := session.Limits(), defaultLimits(); got != want {
		t.Fatalf("Limits() = %+v, want defaults %+v", got, want)
	}

	result := mustExecSession(t, session, "i=0; while true; do i=$((i+1)); done\n")
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126 (loop limit); stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "increase policy.Limits.") {
		t.Fatalf("Stderr = %q, want a default limit message", result.Stderr)
	}

	result = mustExecSession(t, session, "head -c 2000000 /dev/zero\n")
	if got, limit := int64(len(result.Stdout)), defaultLimits().MaxStdoutBytes; got > limit {
		t.Fatalf("len(Stdout) = %d, want capped at %d", got, limit)
	}
	if !result.StdoutTruncated {
		t.Fatal("StdoutTruncated = false, want true")
	}
}

func TestCustomPolicyKeepsExplicitLimitsAndUnlimitedOptOut(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits: policy.Limits{
				MaxCommandCount:   42,
				MaxLoopIterations: policy.Unlimited,
			},
		}),
		LimitOverrides: policy.Limits{MaxGlobOperations: 7, MaxStdoutBytes: policy.Unlimited},
	})

	want := defaultLimits()
	want.MaxCommandCount = 42
	want.MaxLoopIterations = 0
	want.MaxGlobOperations = 7
	want.MaxStdoutBytes = 0
	if got := session.Limits(); got != want {
		t.Fatalf("Limits() = %+v, want %+v", got, want)
	}
}

func TestDefaultPolicyUnlimitedOverrideDisablesLimit(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{
		LimitOverrides: policy.Limits{MaxLoopIterations: policy.Unlimited, MaxCommandCount: policy.Unlimited},
	})
	if got := session.Limits(); got.MaxLoopIterations != 0 || got.MaxCommandCount != 0 {
		t.Fatalf("Limits() = %+v, want loop and command limits disabled", got)
	}
	result := mustExecSession(t, session, "i=0; while [ $i -lt 10005 ]; do i=$((i+1)); done; echo $i\n")
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "10005" {
		t.Fatalf("result = exit %d stdout %q stderr %q, want 10005", result.ExitCode, result.Stdout, result.Stderr)
	}
}
