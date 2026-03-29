package builtins

import (
	"testing"

	"github.com/ewhauser/gbash/shellvariant"
)

func TestDashInvocationUsesPOSIXShellVariant(t *testing.T) {
	t.Parallel()

	inv, err := ParseBashInvocation([]string{"-c", "echo hi"}, BashInvocationConfig{
		Name:             "dash",
		AllowInteractive: true,
	})
	if err != nil {
		t.Fatalf("ParseBashInvocation() error = %v", err)
	}

	if got, want := inv.DefaultShellVariant(), shellvariant.SH; got != want {
		t.Fatalf("DefaultShellVariant() = %q, want %q", got, want)
	}

	req := inv.BuildExecutionRequest(nil, "/", nil, "echo hi")
	if got, want := req.Interpreter, "dash"; got != want {
		t.Fatalf("BuildExecutionRequest().Interpreter = %q, want %q", got, want)
	}
	if got, want := req.ShellVariant, shellvariant.SH; got != want {
		t.Fatalf("BuildExecutionRequest().ShellVariant = %q, want %q", got, want)
	}
}
