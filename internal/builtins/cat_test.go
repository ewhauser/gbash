package builtins_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

func TestCatSupportsLongAndShortNumberFlags(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'alpha\\nbeta\\n' > /tmp/a.txt\nprintf 'gamma\\n' > /tmp/b.txt\ncat --number /tmp/a.txt /tmp/b.txt\ncat -n /tmp/b.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "     1\talpha\n     2\tbeta\n     3\tgamma\n     1\tgamma\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestCatShowEndsHandlesCarriageReturnsAcrossFiles(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf '1\\r' > /tmp/a.txt\nprintf '\\n2\\r\\n' > /tmp/b.txt\ncat -E /tmp/a.txt /tmp/b.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "1^M$\n2^M$\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestCatShowEndsLeavesStandaloneCarriageReturnLiteral(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf '1\\r' > /tmp/a.txt\nprintf '2\\r\\n' > /tmp/b.txt\ncat -E /tmp/a.txt /tmp/b.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "1\r2^M$\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestCatRejectsShellAppendToSelf(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'x\\n' > /tmp/out\ncat /tmp/out >> /tmp/out\nprintf 'status:%s\\n' \"$?\"\ncat /tmp/out\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "status:1\nx\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got, want := result.Stderr, "cat: /tmp/out: input file is output file\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestCatParsesFlagsAfterOperandsLikeGNU(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'a\\n\\n' > /tmp/one.txt\nprintf 'b\\n' > /tmp/two.txt\ncat /tmp/one.txt /tmp/two.txt -s -n\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "     1\ta\n     2\t\n     3\tb\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestCatReportsMissingFilesLikeBash(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "cat missing.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stderr, "cat: missing.txt: No such file or directory\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestCatReportsMissingInputWhenStdoutIsAHostFile(t *testing.T) {
	t.Parallel()
	code, stderr := runCatToHostFile(t, &Config{}, "cat /tmp/missing\n")
	if code != 1 {
		t.Fatalf("ExitCode = %d, want 1", code)
	}
	if want := "cat: /tmp/missing: No such file or directory\n"; stderr != want {
		t.Fatalf("Stderr = %q, want %q", stderr, want)
	}
}

func TestCatReportsPolicyDenialOnceWhenStdoutIsAHostFile(t *testing.T) {
	t.Parallel()
	code, stderr := runCatToHostFile(t, &Config{
		Policy: policy.NewStatic(&policy.Config{ReadRoots: []string{"/tmp", "/usr/bin", "/bin"}}),
	}, "cat /etc/secret\n")
	if code != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", code, stderr)
	}
	if n := strings.Count(stderr, "denied:"); n != 1 {
		t.Fatalf("Stderr = %q, want exactly one denial", stderr)
	}
}

// runCatToHostFile runs script with an os.Pipe as stdout, which takes cat's
// host-file self-overwrite check.
func runCatToHostFile(t *testing.T, cfg *Config, script string) (int, string) {
	t.Helper()
	rt := newRuntime(t, cfg)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	go func() { _, _ = io.Copy(io.Discard, r) }()
	var stderr strings.Builder

	result, err := rt.Run(context.Background(), &ExecutionRequest{Script: script, Stdout: w, Stderr: &stderr})
	_ = w.Close()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return result.ExitCode, stderr.String()
}
