package builtins_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash/policy"
)

// execWithDeadline runs script in a fresh session and fails the test if it
// does not finish within a generous deadline, so regressions that loop or
// allocate without bound surface as failures instead of hangs.
func execWithDeadline(t *testing.T, cfg *Config, script string) *ExecutionResult {
	t.Helper()
	if cfg == nil {
		cfg = &Config{}
	}
	session := newSession(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := session.Exec(ctx, &ExecutionRequest{Script: script})
	if err != nil {
		t.Fatalf("Session.Exec() error = %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("script did not finish before the deadline: %q", script)
	}
	return result
}

func assertExitStderr(t *testing.T, result *ExecutionResult, wantCode int, wantStderr string) {
	t.Helper()
	if result.ExitCode != wantCode {
		t.Fatalf("ExitCode = %d, want %d; stdout=%q stderr=%q", result.ExitCode, wantCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, wantStderr) {
		t.Fatalf("Stderr = %q, want substring %q", result.Stderr, wantStderr)
	}
}

const fieldTooLargeMessage = "field width too large (limit 33554432 bytes)"

func TestODRejectsOversizedWidth(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "echo abc | od -An -tx1 -w200000000000000\n")
	assertExitStderr(t, result, 1, "od: "+fieldTooLargeMessage)
	if result.Stdout != "" {
		t.Fatalf("Stdout = %q, want empty", result.Stdout)
	}
}

func TestODReusesPaddingBufferAcrossLines(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "printf 'abcdefghij' | od -An -tx1 -w4\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, " 61 62 63 64\n 65 66 67 68\n 69 6a\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestTRHighByteRangeTerminates(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "printf 'a\\200b\\377c' | tr -d '\\200-\\377'\nprintf '\\376\\377' | tr '\\376-\\377' 'yz'\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "abcyz"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestTRRejectsOversizedRepeatCounts(t *testing.T) {
	t.Parallel()
	for _, script := range []string{
		"echo abc | tr a '[x*9223372036854775807]'\n",
		"echo abc | tr '[a*4294967296]' x\n",
	} {
		result := execWithDeadline(t, nil, script)
		assertExitStderr(t, result, 1, "tr: string too long (limit 33554432 bytes)")
	}
	result := execWithDeadline(t, nil, "echo abc | tr a '[x*5]'\n")
	if got, want := result.Stdout, "xbc\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q", result.ExitCode, got, want)
	}
}

func TestTextBuiltinsRejectOversizedFieldWidths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		script string
		want   string
	}{
		{"date '+%2147483648Y'\n", "date: " + fieldTooLargeMessage},
		{"date '+%9223372036854775807Y'\n", "date: " + fieldTooLargeMessage},
		{"date '+%99999999999999999999Y'\n", "date: " + fieldTooLargeMessage},
		{"printf 'a\\tb' | expand -t 9223372036854775807\n", "expand: " + fieldTooLargeMessage},
		{"printf 'a\\tb\\tc' | expand -t 1,40000000\n", "expand: " + fieldTooLargeMessage},
		{"echo a | nl -bn -w 4294967296\n", "nl: " + fieldTooLargeMessage},
		{"echo a | nl -w 9223372036854775807\n", "nl: " + fieldTooLargeMessage},
	}
	for _, tc := range cases {
		result := execWithDeadline(t, nil, tc.script)
		assertExitStderr(t, result, 1, tc.want)
		if len(result.Stdout) > 16 {
			t.Fatalf("%q: Stdout = %q, want at most the text before the wide field", tc.script, result.Stdout)
		}
	}
}

func TestDateFieldWidthWithinLimitStillPads(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "date -u -d @0 '+%10Y|%_5d'\n")
	if got, want := result.Stdout, "0000001970|    1\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q; stderr=%q", result.ExitCode, got, want, result.Stderr)
	}
}

func TestColumnCapsFillColumnsAtItemCount(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "echo a | column -c 4294967296\nprintf 'a\\nb\\nc\\n' | column -c 9223372036854775807\nseq 6 | column -c 12\n")
	if got, want := result.Stdout, "a\na  b  c\n1  3  5\n2  4  6\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q; stderr=%q", result.ExitCode, got, want, result.Stderr)
	}
}

func TestExpandWideTabStopStopsOnTimeout(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "printf '\\t\\t\\t\\t\\t\\t\\t\\t' | timeout 0.2 expand -t 30000000 >/dev/null\necho \"rc=$?\"\nprintf 'a\\tb\\n' | expand -t 4\n")
	if got, want := result.Stdout, "rc=0\na   b\n"; got != want && result.Stdout != "rc=124\na   b\n" {
		t.Fatalf("Stdout = %q, want %q (or timeout 124); stderr=%q", got, want, result.Stderr)
	}
}

func TestNLWidePaddingWithinLimit(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "printf 'a\\n\\nb\\n' | nl -w 3 -s '|'\nprintf 'x\\n' | nl -n rz -w 5\n")
	if got, want := result.Stdout, "  1|a\n    \n  2|b\n00001\tx\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q; stderr=%q", result.ExitCode, got, want, result.Stderr)
	}
}

func TestPrintfAndNumfmtHonorWidePrecision(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "printf '%.300000000f' 1\n")
	assertExitStderr(t, result, 126, "printf: value too large (limit 33554432 bytes)")

	result = execWithDeadline(t, nil, strings.Join([]string{
		"printf '%.2000000f' 1.25 | wc -c",
		"printf '%02000000d' -5 | head -c 3; echo",
		"numfmt --format=%.2000000f 1 | wc -c",
		"numfmt --format=%.400f 2.5 | head -c 4; echo",
		"numfmt --format=%010f 5",
		"",
	}, "\n"))
	if result.ExitCode != 0 || strings.Contains(result.Stdout, "%!") {
		t.Fatalf("ExitCode = %d, Stdout = %q; stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if got, want := result.Stdout, "2000002\n-00\n2000003\n2.50\n0000000005\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}

	result = execWithDeadline(t, nil, "numfmt --format=%.40000000f 1\n")
	assertExitStderr(t, result, 1, "numfmt: "+fieldTooLargeMessage)
}

func TestShufWideRangeHeadExitsQuickly(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "shuf -i 0-2000000000 | head -1 | wc -l\nshuf -i 0-9223372036854775807 -n 3 | wc -l\n")
	if got, want := result.Stdout, "1\n3\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q; stderr=%q", result.ExitCode, got, want, result.Stderr)
	}
}

func TestSedCapsSubstitutionGrowth(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "r=$(printf %0200000d 0)\nprintf %010000d 0 > /tmp/in\nsed \"s/x*/$r/g\" /tmp/in\n")
	assertExitStderr(t, result, 1, "sed: pattern space too large (limit 33554432 bytes)")
}

func TestSedStreamingPreservesNewlineRules(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, strings.Join([]string{
		"printf 'a\\nb\\nc' | sed -n -e 2p -e '$p'; echo '|'",
		"printf 'a\\nb\\n' | sed s/a/A/",
		"printf 'a\\nb' | sed 1d; echo '|'",
		"printf 'a\\nb\\n' | sed -n 1p",
		"printf 'x\\n' > /tmp/f; sed -i s/x/y/ /tmp/f; cat /tmp/f",
		"",
	}, "\n"))
	if got, want := result.Stdout, "b\nc|\nA\nb\nb|\na\ny\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q; stderr=%q", result.ExitCode, got, want, result.Stderr)
	}
}

func smallFileLimitConfig(maxFileBytes int64) *Config {
	return &Config{
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits:     policy.Limits{MaxFileBytes: maxFileBytes},
		}),
	}
}

func TestStreamingLineReadersRejectOversizedLines(t *testing.T) {
	t.Parallel()
	for _, cmd := range []string{"uniq", "factor", "numfmt", "tsort"} {
		result := execWithDeadline(t, smallFileLimitConfig(1024), "head -c 4096 /dev/zero | "+cmd+"\n")
		assertExitStderr(t, result, 1, "input exceeds maximum file size of 1024 bytes")
	}
	// Short lines still stream past the per-line limit in total.
	result := execWithDeadline(t, smallFileLimitConfig(1024), "count() { n=0; while read -r _; do n=$((n+1)); done; echo \"$n\"; }\nseq 1000 | uniq | count\nseq 1000 | factor | count\nseq 1000 | numfmt | count\nseq 1000 | tsort | count\n")
	if got, want := result.Stdout, "1000\n1000\n1000\n1000\n"; result.ExitCode != 0 || got != want {
		t.Fatalf("ExitCode = %d, Stdout = %q, want 0 %q; stderr=%q", result.ExitCode, got, want, result.Stderr)
	}
}

func TestFileOperandBuiltinsCapCombinedInput(t *testing.T) {
	t.Parallel()
	// 4 x MaxFileBytes (100) is the combined cap; five 90-byte files exceed it.
	setup := "for i in 1 2 3 4 5 6 7 8 9; do printf 'line%04d\\n' $i; done > /tmp/f\n"
	for _, cmd := range []string{"sort", "sed p", "tac", "paste", "rev", "column"} {
		result := execWithDeadline(t, smallFileLimitConfig(100), setup+cmd+" /tmp/f /tmp/f /tmp/f /tmp/f /tmp/f >/dev/null\n")
		want := 1
		if cmd == "sort" {
			want = 2
		}
		assertExitStderr(t, result, want, "combined input exceeds maximum size of 400 bytes")

		result = execWithDeadline(t, smallFileLimitConfig(100), setup+cmd+" /tmp/f /tmp/f /tmp/f /tmp/f >/dev/null\n")
		if result.ExitCode != 0 {
			t.Fatalf("%s within the cap: ExitCode = %d; stderr=%q", cmd, result.ExitCode, result.Stderr)
		}
	}
}

func TestBasencBase58CapsInputSize(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "head -c 300000 /dev/zero | basenc --base58\n")
	assertExitStderr(t, result, 1, "basenc: base58 input too large (limit 262144 bytes)")
	result = execWithDeadline(t, nil, "head -c 400000 /dev/zero | tr '\\0' 2 | basenc --base58 -d\n")
	assertExitStderr(t, result, 1, "basenc: base58 input too large (limit 393216 bytes)")
}

func TestTailZeroSleepIntervalDoesNotPanic(t *testing.T) {
	t.Parallel()
	result := execWithDeadline(t, nil, "timeout 0.2 tail -s 0 -f /dev/null\necho \"rc=$?\"\n")
	if got, want := result.Stdout, "rc=124\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}
