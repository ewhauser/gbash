package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ewhauser/gbash"
)

func TestReadWriteRootRenamedAndReplacedBySymlinkIsNotMounted(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink swap test requires POSIX symlinks")
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	root := filepath.Join(base, "root")
	secret := filepath.Join(base, "secret")
	for _, dir := range []string{root, secret} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}

	cfg := normalizeConfig(Config{Name: "gbash", SystemTempRoots: func() []string { return []string{base} }})
	rt, err := newRuntime(cfg, &runtimeOptions{readWriteRoot: root})
	if err != nil {
		t.Fatalf("newRuntime() error = %v", err)
	}

	if err := os.Rename(root, filepath.Join(base, "moved")); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if err := os.Symlink(secret, root); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	ctx := context.Background()
	session, err := rt.NewSession(ctx)
	if err == nil {
		_, _ = session.Exec(ctx, &gbash.ExecutionRequest{Script: "echo marker > /marker.txt\n"})
		t.Fatal("NewSession() succeeded after the root was replaced, want error")
	}
	if _, statErr := os.Stat(filepath.Join(secret, "marker.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("marker written to the swapped-in directory (stat err = %v)", statErr)
	}
}

func TestRunRejectsMissingRoot(t *testing.T) {
	t.Parallel()

	cfg := normalizeConfig(Config{Name: "gbash"})
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := newRuntime(cfg, &runtimeOptions{root: missing}); err == nil {
		t.Fatal("newRuntime() with a missing --root succeeded, want error")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := newRuntime(cfg, &runtimeOptions{root: file}); err == nil {
		t.Fatal("newRuntime() with a file --root succeeded, want error")
	}
}

func TestRunRcfileTakesAValue(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	exitCode, stdout, stderr, err := runCLI(t, []string{"--rcfile", "/dev/null", "--root", rootDir, "-c", "pwd"}, "")
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, stderr = %q", exitCode, stderr)
	}
	if got, want := stdout, gbash.DefaultWorkspaceMountPoint+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestParseMaxExecTimeout(t *testing.T) {
	t.Parallel()

	opts, _, err := parseRuntimeOptions([]string{"--server"})
	if err != nil {
		t.Fatalf("parseRuntimeOptions() error = %v", err)
	}
	if opts.maxExecTimeout != defaultMaxExecTimeout {
		t.Fatalf("default maxExecTimeout = %v, want %v", opts.maxExecTimeout, defaultMaxExecTimeout)
	}
	opts, _, err = parseRuntimeOptions([]string{"--server", "--max-exec-timeout", "30s"})
	if err != nil || opts.maxExecTimeout != 30*time.Second {
		t.Fatalf("--max-exec-timeout 30s = %v, %v", opts.maxExecTimeout, err)
	}
	for _, bad := range []string{"--max-exec-timeout=0", "--max-exec-timeout=nope"} {
		if _, _, err := parseRuntimeOptions([]string{"--server", bad}); err == nil {
			t.Fatalf("parseRuntimeOptions(%s) succeeded, want error", bad)
		}
	}
}
