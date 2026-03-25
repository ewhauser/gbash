//go:build !cgo || (darwin && amd64)

package python

import (
	"context"
	"strings"
	"testing"

	gbruntime "github.com/ewhauser/gbash"
)

func TestPythonReportsUnavailableWithoutNativeBindings(t *testing.T) {
	t.Parallel()

	result, err := newPythonRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script: "python -c 'print(\"hi\")'\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "monty native bindings are unavailable") {
		t.Fatalf("Stderr = %q, want gomonty unavailable diagnostic", result.Stderr)
	}
}
