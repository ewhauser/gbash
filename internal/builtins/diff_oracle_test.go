package builtins_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/internal/testutil"
)

type diffOracleResult = oracleCommandResult

func TestDiffMatchesGNUDiff(t *testing.T) {
	t.Parallel()
	diffPath := testutil.RequireNixDiff(t)

	testCases := []struct {
		name  string
		args  []string
		stdin string
		setup func(t *testing.T, workDir string)
	}{
		{
			name: "help",
			args: []string{"--help"},
		},
		{
			name: "version",
			args: []string{"--version"},
		},
		{
			name: "normal-change",
			args: []string{"a.txt", "b.txt"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "a.txt", "one\ntwo\n")
				writeHostFile(t, workDir, "b.txt", "one\nthree\n")
			},
		},
		{
			name: "brief-change",
			args: []string{"--brief", "a.txt", "b.txt"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "a.txt", "one\n")
				writeHostFile(t, workDir, "b.txt", "two\n")
			},
		},
		{
			name: "ignore-case-identical",
			args: []string{"--ignore-case", "--report-identical-files", "a.txt", "b.txt"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "a.txt", "ONE\nTWO\n")
				writeHostFile(t, workDir, "b.txt", "one\ntwo\n")
			},
		},
		{
			name: "unified-labels-no-newline",
			args: []string{"-u", "--label", "LEFT", "--label", "RIGHT", "a.txt", "b.txt"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "a.txt", "one\ntwo")
				writeHostFile(t, workDir, "b.txt", "one\nthree")
			},
		},
		{
			name: "binary-default",
			args: []string{"a.bin", "b.bin"},
			setup: func(t *testing.T, workDir string) {
				writeHostBytes(t, workDir, "a.bin", []byte{'a', 0, 'b', '\n'})
				writeHostBytes(t, workDir, "b.bin", []byte{'a', 0, 'c', '\n'})
			},
		},
		{
			name:  "stdin-unified",
			args:  []string{"-u", "--label", "STDIN", "--label", "FILE", "-", "b.txt"},
			stdin: "one\ntwo\n",
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "b.txt", "one\nthree\n")
			},
		},
		{
			name: "ed-output",
			args: []string{"-e", "a.txt", "b.txt"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "a.txt", "one\ntwo\n")
				writeHostFile(t, workDir, "b.txt", "one\nthree\n")
			},
		},
		{
			name: "rcs-output",
			args: []string{"-n", "a.txt", "b.txt"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "a.txt", "one\ntwo\n")
				writeHostFile(t, workDir, "b.txt", "one\nthree\n")
			},
		},
		{
			name: "ifdef-output",
			args: []string{"-D", "NAME", "a.txt", "b.txt"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "a.txt", "one\ntwo\n")
				writeHostFile(t, workDir, "b.txt", "one\nthree\n")
			},
		},
		{
			name: "recursive-new-file",
			args: []string{"-r", "-N", "dir1", "dir2"},
			setup: func(t *testing.T, workDir string) {
				writeHostFile(t, workDir, "dir1/a.txt", "one\n")
				writeHostFile(t, workDir, "dir2/a.txt", "two\n")
				writeHostFile(t, workDir, "dir2/b.txt", "extra\n")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			workDir := filepath.Join(root, "work")
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) error = %v", workDir, err)
			}
			if tc.setup != nil {
				tc.setup(t, workDir)
			}

			gbash := runGBashDiff(t, root, tc.stdin, tc.args...)
			gnu := runGNUDiff(t, diffPath, workDir, tc.stdin, tc.args...)

			if gbash != gnu {
				t.Fatalf("GNU diff mismatch\ngbash: %+v\ngnu:   %+v", gbash, gnu)
			}
		})
	}
}

func runGBashDiff(t testing.TB, root, stdin string, args ...string) diffOracleResult {
	return runGBashOracleCommand(t, root, stdin, "diff", args...)
}

func runGNUDiff(t testing.TB, diffPath, workDir, stdin string, args ...string) diffOracleResult {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), diffPath, args...) //nolint:gosec // test oracle runs the pinned Nix GNU diff.
	cmd.Args[0] = "diff"
	cmd.Dir = workDir
	cmd.Env = []string{
		"HOME=" + workDir,
		"PWD=" + workDir,
		"PATH=/usr/bin:/bin",
		"LC_ALL=C",
		"LANG=C",
		"TZ=UTC",
		"TMPDIR=" + workDir,
	}
	cmd.Stdin = strings.NewReader(stdin)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("GNU diff Run() error = %v", err)
		}
		exitCode = exitErr.ExitCode()
	}

	return diffOracleResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

func writeHostFile(t testing.TB, workDir, relPath, contents string) {
	t.Helper()
	writeHostBytes(t, workDir, relPath, []byte(contents))
}

func writeHostBytes(t testing.TB, workDir, relPath string, contents []byte) {
	t.Helper()

	absPath := filepath.Join(workDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(absPath), err)
	}
	if err := os.WriteFile(absPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", absPath, err)
	}
}

func diffShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
