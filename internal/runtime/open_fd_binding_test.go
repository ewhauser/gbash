package runtime

import (
	"context"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

func TestRedirectedFDFollowsFileAcrossRenameAndUnlink(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "cd /tmp\n" +
			"exec 3>a; mv a b; echo hi >&3; exec 3>&-; cat b\n" +
			"echo old > c; exec 4<c; rm c; echo new > c; cat <&4; exec 4<&-; cat c\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "hi\nold\nnew\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if result.Stderr != "" {
		t.Fatalf("Stderr = %q, want empty", result.Stderr)
	}
}
