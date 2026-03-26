//go:build linux

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunReadWriteRootTouchDashUsesInheritedStdoutPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "stdout-target")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", target, err)
	}
	old := time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(target, old, old); err != nil {
		t.Fatalf("Chtimes(%q) error = %v", target, err)
	}

	stdoutFile, err := os.Open(target)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", target, err)
	}
	t.Cleanup(func() {
		if closeErr := stdoutFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", target, closeErr)
		}
	})

	var stderr strings.Builder
	exitCode, err := run(
		context.Background(),
		Config{Name: "gbash", SystemTempRoots: func() []string { return []string{os.TempDir()} }},
		[]string{"--readwrite-root", root, "-c", "touch -"},
		strings.NewReader(""),
		stdoutFile,
		&stderr,
		false,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", target, err)
	}
	if !info.ModTime().After(old) {
		t.Fatalf("ModTime(%q) = %v, want after %v", target, info.ModTime(), old)
	}
}

func TestRunReadWriteRootCatDashRejectsInheritedSelfOverwrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "shared.txt")
	if err := os.WriteFile(target, []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", target, err)
	}

	stdinFile, err := os.Open(target)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", target, err)
	}
	t.Cleanup(func() {
		if closeErr := stdinFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", target, closeErr)
		}
	})

	stdoutFile, err := os.OpenFile(target, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", target, err)
	}
	t.Cleanup(func() {
		if closeErr := stdoutFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", target, closeErr)
		}
	})

	var stderr strings.Builder
	exitCode, err := run(
		context.Background(),
		Config{Name: "gbash", SystemTempRoots: func() []string { return []string{os.TempDir()} }},
		[]string{"--readwrite-root", root, "-c", "cat -"},
		stdinFile,
		stdoutFile,
		&stderr,
		false,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1; stderr=%q", exitCode, stderr.String())
	}
	if got, want := stderr.String(), "cat: -: input file is output file\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got, want := string(mustReadFile(t, target)), "keep\n"; got != want {
		t.Fatalf("contents(%q) = %q, want %q", target, got, want)
	}
}

func TestRunReadWriteRootCatDashAllowsInheritedInputAfterConsumption(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "shared.txt")
	if err := os.WriteFile(target, []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", target, err)
	}

	stdinFile, err := os.Open(target)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", target, err)
	}
	t.Cleanup(func() {
		if closeErr := stdinFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", target, closeErr)
		}
	})

	stdoutFile, err := os.OpenFile(target, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", target, err)
	}
	t.Cleanup(func() {
		if closeErr := stdoutFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", target, closeErr)
		}
	})

	var stderr strings.Builder
	exitCode, err := run(
		context.Background(),
		Config{Name: "gbash", SystemTempRoots: func() []string { return []string{os.TempDir()} }},
		[]string{"--readwrite-root", root, "-c", "cat >/dev/null\ncat -"},
		stdinFile,
		stdoutFile,
		&stderr,
		false,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if got, want := string(mustReadFile(t, target)), "keep\n"; got != want {
		t.Fatalf("contents(%q) = %q, want %q", target, got, want)
	}
}

func TestRunReadWriteRootDdExecPreservesInheritedOffsetsAcrossRuns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inputPath := filepath.Join(root, "in.txt")
	outputPath := filepath.Join(root, "out.txt")
	if err := os.WriteFile(inputPath, []byte("abcde\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", inputPath, err)
	}

	stdinFile, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", inputPath, err)
	}
	t.Cleanup(func() {
		if closeErr := stdinFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", inputPath, closeErr)
		}
	})

	stdoutFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", outputPath, err)
	}
	t.Cleanup(func() {
		if closeErr := stdoutFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", outputPath, closeErr)
		}
	})

	cfg := Config{Name: "gbash", SystemTempRoots: func() []string { return []string{os.TempDir()} }}
	runDd := func(args ...string) string {
		t.Helper()

		var stderr strings.Builder
		exitCode, err := run(
			context.Background(),
			cfg,
			append([]string{"--readwrite-root", root, "-c", `exec "$@"`, "_", "dd"}, args...),
			stdinFile,
			stdoutFile,
			&stderr,
			false,
		)
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0; stderr=%q", exitCode, stderr.String())
		}
		return stderr.String()
	}

	if got := runDd("skip=1", "count=1", "bs=1", "status=none"); got != "" {
		t.Fatalf("first stderr = %q, want empty", got)
	}
	if got := runDd("skip=1", "bs=1", "status=none"); got != "" {
		t.Fatalf("second stderr = %q, want empty", got)
	}

	if got, want := string(mustReadFile(t, outputPath)), "bde\n"; got != want {
		t.Fatalf("contents(%q) = %q, want %q", outputPath, got, want)
	}
}

func TestRunReadWriteRootDdCountZeroPreservesInheritedOffsetsAcrossRuns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inputPath := filepath.Join(root, "in.txt")
	outputPath := filepath.Join(root, "out.txt")
	if err := os.WriteFile(inputPath, []byte("LA:3456789abcdef\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", inputPath, err)
	}

	stdinFile, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", inputPath, err)
	}
	t.Cleanup(func() {
		if closeErr := stdinFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", inputPath, closeErr)
		}
	})

	stdoutFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", outputPath, err)
	}
	t.Cleanup(func() {
		if closeErr := stdoutFile.Close(); closeErr != nil {
			t.Fatalf("Close(%q) error = %v", outputPath, closeErr)
		}
	})

	cfg := Config{Name: "gbash", SystemTempRoots: func() []string { return []string{os.TempDir()} }}
	runDd := func(args ...string) string {
		t.Helper()

		var stderr strings.Builder
		exitCode, err := run(
			context.Background(),
			cfg,
			append([]string{"--readwrite-root", root, "-c", `exec "$@"`, "_", "dd"}, args...),
			stdinFile,
			stdoutFile,
			&stderr,
			false,
		)
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0; stderr=%q", exitCode, stderr.String())
		}
		return stderr.String()
	}

	if got := runDd("bs=1", "skip=3", "count=0", "status=none"); got != "" {
		t.Fatalf("first stderr = %q, want empty", got)
	}
	if got := runDd("bs=5", "count=2", "status=none"); got != "" {
		t.Fatalf("second stderr = %q, want empty", got)
	}

	if got, want := string(mustReadFile(t, outputPath)), "3456789abc"; got != want {
		t.Fatalf("contents(%q) = %q, want %q", outputPath, got, want)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return data
}
