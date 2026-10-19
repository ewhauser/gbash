package gbash_test

import (
	"context"
	"testing"

	"github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/policy"
)

func TestExecutionResultReportsSpillTruncation(t *testing.T) {
	t.Parallel()
	rt, err := gbash.New(gbash.WithLimitOverrides(policy.Limits{MaxStdoutBytes: 4}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := rt.Run(context.Background(), &gbash.ExecutionRequest{
		Script:        "printf '%040d' 0\n",
		SpillDir:      t.TempDir(),
		MaxSpillBytes: 10,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.StdoutTruncated || result.StdoutSpillPath == "" || !result.StdoutSpillTruncated {
		t.Fatalf("result = truncated %t spill %q spillTruncated %t, want truncated spill", result.StdoutTruncated, result.StdoutSpillPath, result.StdoutSpillTruncated)
	}
	if result.StderrSpillTruncated {
		t.Fatal("StderrSpillTruncated = true, want false")
	}
}
