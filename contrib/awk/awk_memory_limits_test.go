package awk

import (
	"context"
	"strings"
	"testing"
	"time"

	gbruntime "github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/policy"
)

const (
	awkStringLimitMessage = "awk: string exceeds the 16777216-byte memory limit"
	awkArrayLimitMessage  = "awk: arrays exceed the 268435456-byte memory limit"
)

func runAWKMemoryScript(t *testing.T, rt *gbruntime.Runtime, script string) *gbruntime.ExecutionResult {
	t.Helper()

	start := time.Now()
	result, err := rt.Run(context.Background(), &gbruntime.ExecutionRequest{
		Script:  script,
		Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode == 124 {
		t.Fatalf("awk ran until the timeout after %v; stderr=%q", time.Since(start), result.Stderr)
	}
	return result
}

// These run through the full runtime with the default policy limits, so
// they stick to cases that fail before allocating much. The goawk interp
// package tests cover each bounded operation with small limits.
func TestAWKBoundsRunawayMemory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		program string
		want    string
	}{
		{
			name:    "doubling concat",
			program: `BEGIN { s = "x"; while (1) s = s s }`,
			want:    awkStringLimitMessage,
		},
		{
			name:    "sprintf literal widths",
			program: `BEGIN { f = "%1000000d"; for (i = 0; i < 4; i++) f = f f; s = sprintf(f "%1000000d", 1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17) }`,
			want:    awkStringLimitMessage,
		},
		{
			name:    "printf star widths",
			program: `BEGIN { w = 1000000; printf "%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d%*d", w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1,w,1 }`,
			want:    awkStringLimitMessage,
		},
		{
			name:    "record rebuilt with huge OFS",
			program: `BEGIN { s = "x"; for (i = 0; i < 20; i++) s = s s; OFS = s; $0 = "a"; NF = 100 }`,
			want:    awkStringLimitMessage,
		},
		{
			// Every element shares one 16 MiB string, so the estimate
			// reaches the array budget while real memory stays small.
			name:    "shared large array values",
			program: `BEGIN { s = "x"; for (i = 0; i < 24; i++) s = s s; while (1) a[n++] = s }`,
			want:    awkArrayLimitMessage,
		},
		{
			name:    "split into many elements",
			program: `BEGIN { s = "x"; for (i = 0; i < 22; i++) s = s s; split(s, a, "") }`,
			want:    awkArrayLimitMessage,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runAWKMemoryScript(t, newAWKRuntime(t), "awk '"+tc.program+"'\n")
			if result.ExitCode != 2 {
				t.Fatalf("ExitCode = %d, want 2; stderr=%q", result.ExitCode, result.Stderr)
			}
			if !strings.Contains(result.Stderr, tc.want) {
				t.Fatalf("Stderr = %q, want %q", result.Stderr, tc.want)
			}
			if result.Stdout != "" {
				t.Fatalf("Stdout = %q, want empty", result.Stdout)
			}
		})
	}
}

func TestAWKGensubBoundsResult(t *testing.T) {
	t.Parallel()

	target := strings.Repeat("x", 3000)
	if _, err := awkGensub("x", "&&", "g", target, 4096); err == nil || err.Error() != "gensub: string exceeds the 4096-byte memory limit" {
		t.Fatalf("awkGensub() error = %v, want memory limit error", err)
	}
	if _, err := awkGensub("x+", "&&", "g", target, 4096); err == nil {
		t.Fatalf("awkGensub() with one large match: error = nil, want memory limit error")
	}
	got, err := awkGensub("x", "&&", "g", target[:2048], 4096)
	if err != nil || len(got) != 4096 {
		t.Fatalf("awkGensub() = %d bytes, %v; want 4096 bytes", len(got), err)
	}
	got, err = awkGensub("(x)", `[\1]`, "2", "xxx", 4096)
	if err != nil || got != "x[x]x" {
		t.Fatalf("awkGensub() = %q, %v; want %q", got, err, "x[x]x")
	}
}

func TestAWKStringLimitFollowsPolicyLimits(t *testing.T) {
	t.Parallel()

	// 24 MiB fits under a 32 MiB file limit but not under the 16 MiB floor.
	program := `awk 'BEGIN { s = "x"; for (i = 0; i < 23; i++) s = s s; t = s s s; print length(t) }'` + "\n"

	result := runAWKMemoryScript(t, newAWKRuntime(t), program)
	if result.ExitCode != 2 || !strings.Contains(result.Stderr, awkStringLimitMessage) {
		t.Fatalf("default limits: ExitCode = %d, Stderr = %q; want exit 2 with %q", result.ExitCode, result.Stderr, awkStringLimitMessage)
	}

	rt, err := gbruntime.New(gbruntime.WithConfig(&gbruntime.Config{
		Registry:       newAWKRegistry(t),
		LimitOverrides: policy.Limits{MaxFileBytes: 32 << 20},
	}))
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}
	result = runAWKMemoryScript(t, rt, program)
	if result.ExitCode != 0 {
		t.Fatalf("32 MiB file limit: ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "25165824\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
