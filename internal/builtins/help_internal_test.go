package builtins

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func runHelpCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	err := NewHelp().Run(context.Background(), &Invocation{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Env: map[string]string{
			archEnvKey:         "aarch64",
			unameReleaseEnvKey: "25.2.0",
		},
	})
	return stdout.String(), stderr.String(), err
}

func TestHelpDefaultOutputMatchesBashListShape(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runHelpCommand(t)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "GNU bash, version 5.3.9(1)-release (aarch64-apple-darwin25.2.0)\n" + bashHelpListBody
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestHelpDetailedTopicMatchesBashHelpBuiltin(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runHelpCommand(t, "help")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout, builtinHelp["help"].Body; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestHelpShortSynopsisMatchesBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runHelpCommand(t, "-s", "pwd")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout, "pwd: pwd [-LP]\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestHelpModePrecedenceMatchesBash(t *testing.T) {
	t.Parallel()

	describeOut, stderr, err := runHelpCommand(t, "-d", "help")
	if err != nil {
		t.Fatalf("Run(-d) error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, args := range [][]string{
		{"-s", "-d", "help"},
		{"-d", "-s", "help"},
		{"-d", "-m", "help"},
		{"-m", "-d", "help"},
		{"-sd", "help"},
		{"-md", "help"},
	} {
		stdout, stderr, err := runHelpCommand(t, args...)
		if err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		if stdout != describeOut {
			t.Fatalf("stdout for %v = %q, want %q", args, stdout, describeOut)
		}
		if stderr != "" {
			t.Fatalf("stderr for %v = %q, want empty", args, stderr)
		}
	}

	manpageOut, stderr, err := runHelpCommand(t, "-m", "help")
	if err != nil {
		t.Fatalf("Run(-m) error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, args := range [][]string{
		{"-m", "-s", "help"},
		{"-s", "-m", "help"},
		{"-ms", "help"},
	} {
		stdout, stderr, err := runHelpCommand(t, args...)
		if err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		if stdout != manpageOut {
			t.Fatalf("stdout for %v = %q, want %q", args, stdout, manpageOut)
		}
		if stderr != "" {
			t.Fatalf("stderr for %v = %q, want empty", args, stderr)
		}
	}
}

func TestHelpRejectsInvalidOptionsLikeBash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "short",
			args: []string{"-z", "help"},
			want: "help: -z: invalid option\nhelp: usage: help [-dms] [pattern ...]\n",
		},
		{
			name: "clustered",
			args: []string{"-sz", "help"},
			want: "help: -z: invalid option\nhelp: usage: help [-dms] [pattern ...]\n",
		},
		{
			name: "long",
			args: []string{"--foo", "help"},
			want: "help: --: invalid option\nhelp: usage: help [-dms] [pattern ...]\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runHelpCommand(t, tc.args...)
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}

			var exitErr *ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("error = %v, want ExitError", err)
			}
			if exitErr.Code != 2 {
				t.Fatalf("exit code = %d, want 2", exitErr.Code)
			}
			if stderr != tc.want {
				t.Fatalf("stderr = %q, want %q", stderr, tc.want)
			}
		})
	}
}
