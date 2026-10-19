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
			name:    "multi concat",
			program: `BEGIN { s = "x"; while (1) s = s "-" s }`,
			want:    awkStringLimitMessage,
		},
		{
			name:    "gsub amplification",
			program: `BEGIN { s = "x"; while (1) gsub(/x/, "&&", s) }`,
			want:    awkStringLimitMessage,
		},
		{
			name:    "gsub single huge replacement",
			program: `BEGIN { s = "x"; for (i = 0; i < 13; i++) s = s s; r = s; gsub(/&/, "\\&", r); gsub(/x/, r, s) }`,
			want:    awkStringLimitMessage,
		},
		{
			name:    "gensub amplification",
			program: `BEGIN { s = "x"; while (1) s = gensub("x+", "&&", "g", s) }`,
			want:    "awk: gensub: string exceeds the 16777216-byte memory limit",
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
			name:    "distinct large array values",
			program: `BEGIN { s = "x"; for (i = 0; i < 20; i++) s = s s; while (1) a[n++] = s n }`,
			want:    awkArrayLimitMessage,
		},
		{
			name:    "many array elements",
			program: `BEGIN { while (1) a[n++] }`,
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

func TestAWKMemoryLimitsCreditFreedArrays(t *testing.T) {
	t.Parallel()

	// Each pass holds about 80 MiB of array values and then frees them, so
	// the passes would add up to far more than the array budget if freed
	// elements were not credited back.
	result := runAWKMemoryScript(t, newAWKRuntime(t), `awk '
function fill(n, local) { for (i = 0; i < 10; i++) local[i] = s i n; return length(local) }
BEGIN {
	s = "x"; for (i = 0; i < 23; i++) s = s s
	for (pass = 0; pass < 10; pass++) {
		for (i = 0; i < 10; i++) a[i] = s i pass
		delete a[0]
		delete a
		total += fill(pass)
		total += split(s s, b, "q")
	}
	print length(s), total
}'
`)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "8388608 110\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
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
