package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ewhauser/gbash"
)

func TestRunCommandStringTreatsTrailingFlagsAsPositionals(t *testing.T) {
	t.Parallel()

	hostRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostRoot, "secret.txt"), []byte("host-secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdoutBuf, stderrBuf strings.Builder
	exitCode, err := run(context.Background(), Config{Name: "gbash"}, []string{
		"-c",
		`printf '0=%s 1=%s 2=%s 3=%s\n' "$0" "$1" "$2" "$3"; pwd; cat /home/agent/project/secret.txt 2>/dev/null || echo no-mount`,
		"--root=" + hostRoot,
		"--json",
		"--readwrite-root",
		"-x",
	}, strings.NewReader(""), &stdoutBuf, &stderrBuf, false)
	stdout, stderr := stdoutBuf.String(), stderrBuf.String()
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0; stderr=%q", exitCode, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	want := "0=--root=" + hostRoot + " 1=--json 2=--readwrite-root 3=-x\n/home/agent\nno-mount\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunCommandStringKeepsLiteralDoubleDashPositional(t *testing.T) {
	t.Parallel()

	exitCode, stdout, stderr, err := runCLI(t, []string{"-c", `printf '%s|%s\n' "$0" "$1"`, "--", "a"}, "")
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 || stderr != "" {
		t.Fatalf("exitCode = %d, stderr = %q", exitCode, stderr)
	}
	if got, want := stdout, "--|a\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunCommandStringStillHonorsLeadingFlags(t *testing.T) {
	t.Parallel()

	exitCode, stdout, stderr, err := runCLI(t, []string{"--json", "-o", "pipefail", "-c", `echo "$0"`, "name"}, "")
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %q", exitCode, stderr)
	}
	if !strings.Contains(stdout, `"stdout":"name\n"`) {
		t.Fatalf("stdout = %q, want JSON result with $0=name", stdout)
	}
}

func TestRunScriptFileTreatsTrailingFlagsAsPositionals(t *testing.T) {
	t.Parallel()

	rootDir := writeCLIRootScript(t, "main.sh", `printf '%s|%s\n' "$1" "$2"`+"\n")
	exitCode, stdout, stderr, err := runCLI(t, []string{"--root", rootDir, "main.sh", "--root=/", "--json"}, "")
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 || stderr != "" {
		t.Fatalf("exitCode = %d, stderr = %q", exitCode, stderr)
	}
	if got, want := stdout, "--root=/|--json\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestReadWriteRootSymlinkSwapDoesNotRedirectLaterSessions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink swap test requires POSIX symlinks")
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	trusted := filepath.Join(base, "trusted")
	inside := filepath.Join(trusted, "work")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{inside, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	cfg := normalizeConfig(Config{
		Name:            "gbash",
		SystemTempRoots: func() []string { return []string{trusted} },
	})
	rt, err := newRuntime(cfg, &runtimeOptions{readWriteRoot: link})
	if err != nil {
		t.Fatalf("newRuntime() error = %v", err)
	}

	// Swap the symlink to an untrusted directory after validation.
	if err := os.Remove(link); err != nil {
		t.Fatalf("Remove(link) error = %v", err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("Symlink(outside) error = %v", err)
	}

	ctx := context.Background()
	session, err := rt.NewSession(ctx)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	result, err := session.Exec(ctx, &gbash.ExecutionRequest{Script: "echo marker > /marker.txt\n"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, stderr = %q", result.ExitCode, result.Stderr)
	}
	if _, err := os.Stat(filepath.Join(outside, "marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("marker written to swapped symlink target (stat err = %v)", err)
	}
	data, err := os.ReadFile(filepath.Join(inside, "marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(inside marker) error = %v", err)
	}
	if got, want := string(data), "marker\n"; got != want {
		t.Fatalf("marker = %q, want %q", got, want)
	}
}
