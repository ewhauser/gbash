package main

import (
	"bytes"
	"context"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func BenchmarkCLIBinary(b *testing.B) {
	binaryPath := buildCLIBenchmarkBinary(b)

	cases := []struct {
		name       string
		args       []string
		stdin      string
		wantStdout string
	}{
		{
			name:       "EmptyScript",
			stdin:      "",
			wantStdout: "",
		},
		{
			name:       "SimpleScript",
			stdin:      "echo hi\n",
			wantStdout: "hi\n",
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			stdout, stderr, err := runCLIBenchmarkProcess(binaryPath, tc.args, tc.stdin, nil, nil)
			if err != nil {
				b.Fatalf("verify run error = %v; stderr=%q", err, stderr)
			}
			if got := stdout; got != tc.wantStdout {
				b.Fatalf("verify stdout = %q, want %q", got, tc.wantStdout)
			}
			if got := stderr; got != "" {
				b.Fatalf("verify stderr = %q, want empty stderr", got)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := runCLIBenchmarkProcess(binaryPath, tc.args, tc.stdin, io.Discard, io.Discard); err != nil {
					b.Fatalf("run error = %v", err)
				}
			}
		})
	}
}

func buildCLIBenchmarkBinary(tb testing.TB) string {
	tb.Helper()

	tmp := tb.TempDir()
	name := "gbash"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binaryPath := filepath.Join(tmp, name)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := osexec.CommandContext(ctx, "go", "build", "-o", binaryPath, ".")
	cmd.Dir = benchmarkCLIWorkDir(tb)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		tb.Fatalf("go build timed out: %v", ctx.Err())
	}
	if err != nil {
		tb.Fatalf("go build error = %v\n%s", err, output)
	}
	return binaryPath
}

func benchmarkCLIWorkDir(tb testing.TB) string {
	tb.Helper()

	dir, err := os.Getwd()
	if err != nil {
		tb.Fatalf("Getwd() error = %v", err)
	}
	return dir
}

func runCLIBenchmarkProcess(binaryPath string, args []string, stdinText string, stdoutSink, stderrSink io.Writer) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, binaryPath, args...)
	cmd.Stdin = bytes.NewBufferString(stdinText)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	if stdoutSink == nil {
		cmd.Stdout = &stdoutBuf
	} else {
		cmd.Stdout = stdoutSink
	}
	if stderrSink == nil {
		cmd.Stderr = &stderrBuf
	} else {
		cmd.Stderr = stderrSink
	}

	err = cmd.Run()
	if ctx.Err() != nil {
		return stdoutBuf.String(), stderrBuf.String(), ctx.Err()
	}
	return stdoutBuf.String(), stderrBuf.String(), err
}
