package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

func TestDefaultSandboxLayout(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo \"$HOME\"\necho \"$PATH\"\nls /\nls /bin\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) < 2 {
		t.Fatalf("Stdout = %q, want at least two lines", result.Stdout)
	}
	if got, want := lines[0], defaultHomeDir; got != want {
		t.Fatalf("HOME = %q, want %q", got, want)
	}
	if got, want := lines[1], defaultPath; got != want {
		t.Fatalf("PATH = %q, want %q", got, want)
	}

	for _, entry := range []string{"bin", "home", "tmp", "usr"} {
		if !containsLine(lines, entry) {
			t.Fatalf("Stdout missing root entry %q: %q", entry, result.Stdout)
		}
	}
	for _, entry := range []string{"cat", "echo", "ls", "mkdir", "pwd", "rm"} {
		if !containsLine(lines, entry) {
			t.Fatalf("Stdout missing /bin stub %q: %q", entry, result.Stdout)
		}
	}
	if containsLine(lines, "__jb_cd_resolve") {
		t.Fatalf("Stdout should not expose internal command stubs: %q", result.Stdout)
	}
}

func TestWorkDirUpdatesPWD(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		WorkDir: "/tmp",
		Script:  "echo \"$PWD\"\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "/tmp\n/tmp\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestRelativePathsUseVirtualWorkDir(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo hi > note.txt\ncat note.txt\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "hi\n/home/agent\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestVirtualCDUpdatesPWD(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "pwd\ncd /tmp\npwd\ncd \"$HOME\"\npwd\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "/home/agent\n/tmp\n/home/agent\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestPwdHonorsLogicalAndPhysicalModes(t *testing.T) {
	rt := newRuntime(t, &Config{
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/"},
			WriteRoots:  []string{"/"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"mkdir -p a/b\n" +
			"ln -s a/b c\n" +
			"cd c\n" +
			"pwd -L\n" +
			"pwd -P\n" +
			"pwd\n" +
			"POSIXLY_CORRECT=1 pwd\n" +
			"PWD=\"$PWD/.\" pwd -L\n" +
			"PWD=bogus pwd -L\n" +
			"PWD=\"/home/agent\" pwd -L\n" +
			"PWD=\"/home/agent/a/../c\" pwd -L\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	want := "" +
		"/home/agent/c\n" +
		"/home/agent/a/b\n" +
		"/home/agent/a/b\n" +
		"/home/agent/c\n" +
		"/home/agent/a/b\n" +
		"/home/agent/a/b\n" +
		"/home/agent/a/b\n" +
		"/home/agent/a/b\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
