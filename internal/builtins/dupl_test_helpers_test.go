package builtins_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gbruntime "github.com/ewhauser/gbash"
	gbfs "github.com/ewhauser/gbash/fs"
	"github.com/ewhauser/gbash/policy"
)

type oracleCommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type commandBehaviorTestCase struct {
	name            string
	script          string
	wantCode        int
	wantOut         string
	wantOutContains []string
	wantStderr      string
}

func runCommandBehaviorCases(t *testing.T, rt *Runtime, tests []commandBehaviorTestCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := rt.Run(context.Background(), &ExecutionRequest{Script: tc.script})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.ExitCode != tc.wantCode {
				t.Fatalf("ExitCode = %d, want %d; stderr=%q", result.ExitCode, tc.wantCode, result.Stderr)
			}
			if got := result.Stdout; len(tc.wantOutContains) > 0 {
				for _, want := range tc.wantOutContains {
					if !strings.Contains(got, want) {
						t.Fatalf("Stdout = %q, want to contain %q", got, want)
					}
				}
			} else if got != tc.wantOut {
				t.Fatalf("Stdout = %q, want %q", got, tc.wantOut)
			}
			if got := result.Stderr; got != tc.wantStderr {
				t.Fatalf("Stderr = %q, want %q", got, tc.wantStderr)
			}
		})
	}
}

func runHelpAndVersionShortCircuitTest(t *testing.T, command, helpScript, helpWant, versionScript, versionWant string) {
	t.Helper()

	rt := newRuntime(t, &Config{})

	helpResult, err := rt.Run(context.Background(), &ExecutionRequest{Script: helpScript})
	if err != nil {
		t.Fatalf("Run(help) error = %v", err)
	}
	if helpResult.ExitCode != 0 {
		t.Fatalf("help ExitCode = %d, want 0; stderr=%q", helpResult.ExitCode, helpResult.Stderr)
	}
	if !strings.Contains(helpResult.Stdout, helpWant) {
		t.Fatalf("help Stdout = %q, want %q", helpResult.Stdout, helpWant)
	}

	versionResult, err := rt.Run(context.Background(), &ExecutionRequest{Script: versionScript})
	if err != nil {
		t.Fatalf("Run(version) error = %v", err)
	}
	if versionResult.ExitCode != 0 {
		t.Fatalf("%s version ExitCode = %d, want 0; stderr=%q", command, versionResult.ExitCode, versionResult.Stderr)
	}
	if !strings.Contains(versionResult.Stdout, versionWant) {
		t.Fatalf("%s version Stdout = %q, want %q", command, versionResult.Stdout, versionWant)
	}
}

func newAllowedHTTPRuntime(t *testing.T, handler http.HandlerFunc, cfg func(*NetworkConfig)) (*Runtime, string) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	network := &NetworkConfig{
		AllowedURLPrefixes: []string{server.URL},
		DenyPrivateRanges:  false,
	}
	if cfg != nil {
		cfg(network)
	}

	return newRuntime(t, &Config{Network: network}), server.URL
}

func runEnvSymlinkCommandTest(t *testing.T, targetName, scriptBody, linkName, commandScript, wantOut string) {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, targetName), []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", targetName, err)
	}
	if err := os.Symlink(targetName, filepath.Join(root, linkName)); err != nil {
		t.Fatalf("Symlink(%s) error = %v", linkName, err)
	}

	rt := newRuntime(t, &Config{
		FileSystem: gbruntime.ReadWriteDirectoryFileSystem(root, gbruntime.ReadWriteDirectoryOptions{}),
	})
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		WorkDir: "/",
		Script: "PATH=/bin:/usr/bin:\n" +
			"export PATH\n" +
			commandScript,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if got := result.Stdout; got != wantOut {
		t.Fatalf("Stdout = %q, want %q", got, wantOut)
	}
	if got := result.Stderr; got != "" {
		t.Fatalf("Stderr = %q, want empty", got)
	}
}

func runGBashOracleCommand(t testing.TB, root, stdin, command string, args ...string) oracleCommandResult {
	t.Helper()

	env := defaultBaseEnv()
	env["HOME"] = "/"
	env["LC_ALL"] = "C"
	env["LANG"] = "C"
	env["TZ"] = "UTC"

	rt := newRuntime(t, &Config{
		BaseEnv:    env,
		FileSystem: gbruntime.ReadWriteDirectoryFileSystem(root, gbruntime.ReadWriteDirectoryOptions{}),
	})

	var script strings.Builder
	script.WriteString(command)
	for _, arg := range args {
		script.WriteByte(' ')
		script.WriteString(diffShellQuote(arg))
	}
	script.WriteByte('\n')

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		WorkDir: "/work",
		Script:  script.String(),
		Stdin:   strings.NewReader(stdin),
	})
	if err != nil {
		t.Fatalf("gbash Run(%q) error = %v", script.String(), err)
	}

	return oracleCommandResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}
}

type stderrContainsCase struct {
	name       string
	script     string
	wantCode   int
	wantStderr string
}

func runStderrContainsCases(t *testing.T, rt *Runtime, tests []stderrContainsCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := rt.Run(context.Background(), &ExecutionRequest{Script: tc.script})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.ExitCode != tc.wantCode {
				t.Fatalf("ExitCode = %d, want %d; stderr=%q", result.ExitCode, tc.wantCode, result.Stderr)
			}
			if !strings.Contains(result.Stderr, tc.wantStderr) {
				t.Fatalf("Stderr = %q, want %q", result.Stderr, tc.wantStderr)
			}
		})
	}
}

type exactStderrCase struct {
	name       string
	script     string
	wantCode   int
	wantStderr string
}

func runExactStderrCases(t *testing.T, rt *Runtime, tests []exactStderrCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := rt.Run(context.Background(), &ExecutionRequest{Script: tc.script})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.ExitCode != tc.wantCode {
				t.Fatalf("ExitCode = %d, want %d; stderr=%q", result.ExitCode, tc.wantCode, result.Stderr)
			}
			if got := result.Stderr; got != tc.wantStderr {
				t.Fatalf("Stderr = %q, want %q", got, tc.wantStderr)
			}
		})
	}
}

func newInfoCountingSession(t *testing.T) (*Session, *infoCountingFS) {
	t.Helper()

	var tracked *infoCountingFS
	session := newSession(t, &Config{
		FileSystem: CustomFileSystem(gbfs.FactoryFunc(func(context.Context) (gbfs.FileSystem, error) {
			tracked = &infoCountingFS{FileSystem: gbfs.NewMemory()}
			return tracked, nil
		}), defaultHomeDir),
	})
	return session, tracked
}

type sessionStdoutCase struct {
	name   string
	script string
	want   string
}

func runSessionStdoutCases(t *testing.T, tests []sessionStdoutCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := newSession(t, &Config{})
			result := mustExecSession(t, session, tc.script)
			if result.ExitCode != 0 {
				t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
			}
			if got := result.Stdout; got != tc.want {
				t.Fatalf("Stdout = %q, want %q", got, tc.want)
			}
			if result.Stderr != "" {
				t.Fatalf("Stderr = %q, want empty", result.Stderr)
			}
		})
	}
}

type transformCheck struct {
	name   string
	script string
	want   string
}

func runTransformChecks(t *testing.T, inputPath string, input []byte, checks []transformCheck) {
	t.Helper()

	session := newSession(t, &Config{})
	writeSessionFile(t, session, inputPath, input)

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := mustExecSession(t, session, tc.script)
			if result.ExitCode != 0 {
				t.Fatalf("%s ExitCode = %d, want 0; stderr=%q", tc.name, result.ExitCode, result.Stderr)
			}
			if got := result.Stdout; got != tc.want {
				t.Fatalf("%s stdout = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func newReadWriteFollowSession(t *testing.T) *Session {
	t.Helper()

	return newSession(t, &Config{
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/"},
			WriteRoots:  []string{"/"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})
}

func assertTransformErrorScenarios(
	t *testing.T,
	commandName string,
	input []byte,
	errorCases []exactStderrCase,
	multiScript string,
	wantStdout string,
	wantStderrContains []string,
) {
	t.Helper()

	rt := newRuntime(t, &Config{})
	runExactStderrCases(t, rt, errorCases)

	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/ok.txt", input)
	result := mustExecSession(t, session, multiScript)
	if result.ExitCode != 1 {
		t.Fatalf("%s multiResult ExitCode = %d, want 1; stderr=%q", commandName, result.ExitCode, result.Stderr)
	}
	if got := result.Stdout; got != wantStdout {
		t.Fatalf("%s multiResult stdout = %q, want %q", commandName, got, wantStdout)
	}
	for _, want := range wantStderrContains {
		if !strings.Contains(result.Stderr, want) {
			t.Fatalf("%s multiResult stderr = %q, want substring %q", commandName, result.Stderr, want)
		}
	}
}
