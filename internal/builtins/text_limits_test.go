package builtins_test

import (
	"context"
	"strings"
	"testing"
	"time"
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
