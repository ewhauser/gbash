package runtime

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLSSupportsHelpDirectoryAndHumanReadableFlags(t *testing.T) {
	session := newSession(t, &Config{})

	writeSessionFile(t, session, "/home/agent/local.txt", []byte("should not appear\n"))
	if err := session.FileSystem().MkdirAll(context.Background(), "/tmp/lsdir/sub", 0o755); err != nil {
		t.Fatalf("MkdirAll(lsdir/sub) error = %v", err)
	}
	writeSessionFile(t, session, "/tmp/lsdir/big.bin", bytes.Repeat([]byte("x"), 2048))

	result := mustExecSession(t, session,
		"ls --help\n"+
			"echo ---\n"+
			"ls -ld /tmp/lsdir\n"+
			"echo ---\n"+
			"ls -h -l /tmp/lsdir/big.bin\n",
	)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	parts := strings.Split(result.Stdout, "---\n")
	if len(parts) != 3 {
		t.Fatalf("Stdout blocks = %q, want 3 blocks", result.Stdout)
	}

	helpBlock := parts[0]
	if !strings.Contains(helpBlock, "Usage:\n  ls [OPTION]... [FILE]...") {
		t.Fatalf("help block = %q, want usage text", helpBlock)
	}
	if strings.Contains(helpBlock, "local.txt") {
		t.Fatalf("help block = %q, want no directory listing after help", helpBlock)
	}

	if got := strings.TrimSpace(parts[1]); !strings.HasSuffix(got, "/tmp/lsdir/") {
		t.Fatalf("directory listing = %q, want directory suffix", got)
	}

	humanLine := strings.TrimSpace(parts[2])
	if !strings.Contains(humanLine, "2.0K") {
		t.Fatalf("human-readable listing = %q, want 2.0K size", humanLine)
	}
	if !strings.HasSuffix(humanLine, "/tmp/lsdir/big.bin") {
		t.Fatalf("human-readable listing = %q, want big.bin path", humanLine)
	}
}

func TestLSSupportsHiddenAndClassifyFlags(t *testing.T) {
	session := newSession(t, &Config{})

	writeSessionFile(t, session, "/tmp/classify/.hidden", []byte("secret\n"))
	writeSessionFile(t, session, "/tmp/classify/plain.txt", []byte("plain\n"))
	writeSessionFile(t, session, "/tmp/classify/run.sh", []byte("#!/bin/sh\n"))
	writeSessionFile(t, session, "/tmp/classify/sub/nested.txt", []byte("nested\n"))
	if err := session.FileSystem().Chmod(context.Background(), "/tmp/classify/run.sh", 0o755); err != nil {
		t.Fatalf("Chmod(run.sh) error = %v", err)
	}

	result := mustExecSession(t, session,
		"ls /tmp/classify\n"+
			"echo ---\n"+
			"ls -a /tmp/classify\n"+
			"echo ---\n"+
			"ls -A /tmp/classify\n"+
			"echo ---\n"+
			"ls -F /tmp/classify\n",
	)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	parts := strings.Split(result.Stdout, "---\n")
	if len(parts) != 4 {
		t.Fatalf("Stdout blocks = %q, want 4 blocks", result.Stdout)
	}

	defaultLines := splitTrimmedLines(parts[0])
	if slices.Contains(defaultLines, ".hidden") || slices.Contains(defaultLines, ".") || slices.Contains(defaultLines, "..") {
		t.Fatalf("default ls lines = %v, want hidden entries omitted", defaultLines)
	}

	allLines := splitTrimmedLines(parts[1])
	for _, want := range []string{".", "..", ".hidden"} {
		if !slices.Contains(allLines, want) {
			t.Fatalf("ls -a lines = %v, want %q", allLines, want)
		}
	}

	almostAllLines := splitTrimmedLines(parts[2])
	if !slices.Contains(almostAllLines, ".hidden") {
		t.Fatalf("ls -A lines = %v, want hidden file", almostAllLines)
	}
	if slices.Contains(almostAllLines, ".") || slices.Contains(almostAllLines, "..") {
		t.Fatalf("ls -A lines = %v, want no dot entries", almostAllLines)
	}

	classifiedLines := splitTrimmedLines(parts[3])
	for _, want := range []string{"run.sh*", "sub/"} {
		if !slices.Contains(classifiedLines, want) {
			t.Fatalf("ls -F lines = %v, want %q", classifiedLines, want)
		}
	}
}

func TestLSSupportsSortingAndRecursiveFlags(t *testing.T) {
	session := newSession(t, &Config{})

	writeSessionFile(t, session, "/tmp/sortsize/a.txt", []byte("a"))
	writeSessionFile(t, session, "/tmp/sortsize/b.txt", bytes.Repeat([]byte("b"), 2048))
	writeSessionFile(t, session, "/tmp/sortsize/c.txt", []byte("1234567890"))
	writeSessionFile(t, session, "/tmp/sorttime/old.txt", []byte("old"))
	writeSessionFile(t, session, "/tmp/sorttime/new.txt", []byte("new"))
	writeSessionFile(t, session, "/tmp/recurse/root.txt", []byte("root"))
	writeSessionFile(t, session, "/tmp/recurse/sub/nested.txt", []byte("nested"))

	now := time.Now().UTC()
	old := now.Add(-2 * time.Hour)
	for _, item := range []struct {
		path string
		when time.Time
	}{
		{"/tmp/sorttime/old.txt", old},
		{"/tmp/sorttime/new.txt", now},
	} {
		if err := session.FileSystem().Chtimes(context.Background(), item.path, item.when, item.when); err != nil {
			t.Fatalf("Chtimes(%q) error = %v", item.path, err)
		}
	}

	result := mustExecSession(t, session,
		"ls -S /tmp/sortsize\n"+
			"echo ---\n"+
			"ls -rt /tmp/sorttime\n"+
			"echo ---\n"+
			"ls -R /tmp/recurse\n",
	)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	parts := strings.Split(result.Stdout, "---\n")
	if len(parts) != 3 {
		t.Fatalf("Stdout blocks = %q, want 3 blocks", result.Stdout)
	}

	if got, want := splitTrimmedLines(parts[0]), []string{"b.txt", "c.txt", "a.txt"}; !slices.Equal(got, want) {
		t.Fatalf("ls -S lines = %v, want %v", got, want)
	}

	if got, want := splitTrimmedLines(parts[1]), []string{"old.txt", "new.txt"}; !slices.Equal(got, want) {
		t.Fatalf("ls -rt lines = %v, want %v", got, want)
	}

	recursive := parts[2]
	for _, want := range []string{"/tmp/recurse:\n", "/tmp/recurse/sub:\n", "root.txt\n", "nested.txt\n"} {
		if !strings.Contains(recursive, want) {
			t.Fatalf("recursive output = %q, want %q", recursive, want)
		}
	}
}

func TestLSReturnsMissingPathExitCode(t *testing.T) {
	session := newSession(t, &Config{})

	result := mustExecSession(t, session, "ls /tmp/missing\n")
	if result.ExitCode != 2 {
		t.Fatalf("ExitCode = %d, want 2", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "ls: /tmp/missing: No such file or directory") {
		t.Fatalf("Stderr = %q, want missing-path error", result.Stderr)
	}
}

func TestDirUsesDirUsageAndNonLongDefaultOutput(t *testing.T) {
	session := newSession(t, &Config{})

	writeSessionFile(t, session, "/tmp/dir-view/alpha.txt", []byte("alpha\n"))
	writeSessionFile(t, session, "/tmp/dir-view/beta.txt", []byte("beta\n"))

	result := mustExecSession(t, session, "dir --help\necho ---\ndir /tmp/dir-view\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	parts := strings.Split(result.Stdout, "---\n")
	if len(parts) != 2 {
		t.Fatalf("Stdout blocks = %q, want 2 blocks", result.Stdout)
	}
	helpBlock := parts[0]
	if !strings.Contains(helpBlock, "Usage:\n  dir [OPTION]... [FILE]...") {
		t.Fatalf("help block = %q, want dir usage", helpBlock)
	}
	if strings.Contains(helpBlock, "ls [OPTION]") {
		t.Fatalf("help block = %q, want no ls usage", helpBlock)
	}

	listing := strings.TrimSpace(parts[1])
	for _, want := range []string{"alpha.txt", "beta.txt"} {
		if !strings.Contains(listing, want) {
			t.Fatalf("dir output = %q, want %q", listing, want)
		}
	}
	if strings.Contains(listing, "-rw") {
		t.Fatalf("dir output = %q, want non-long default output", listing)
	}
}

func TestDirSupportsLongFormatViaLSFlags(t *testing.T) {
	session := newSession(t, &Config{})

	writeSessionFile(t, session, "/tmp/dir-long/item.txt", []byte("payload\n"))

	result := mustExecSession(t, session, "dir -l /tmp/dir-long\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "item.txt") || !strings.Contains(result.Stdout, "-rw") {
		t.Fatalf("Stdout = %q, want long-format output", result.Stdout)
	}
}

func splitTrimmedLines(block string) []string {
	trimmed := strings.TrimSpace(block)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
