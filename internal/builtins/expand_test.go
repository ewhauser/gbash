package builtins_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestExpandHelpAndVersion(t *testing.T) {
	t.Parallel()

	runHelpAndVersionShortCircuitTest(
		t,
		"expand",
		"expand --help --tabs=x\n",
		"Usage: expand [OPTION]... [FILE]...",
		"expand --version --tabs=x\n",
		"expand (gbash)\n",
	)

	rt := newRuntime(t, &Config{})
	helpResult, err := rt.Run(context.Background(), &ExecutionRequest{Script: "expand --help --tabs=x\n"})
	if err != nil {
		t.Fatalf("Run(help) error = %v", err)
	}
	for _, want := range []string{
		"  -i, --initial    do not convert tabs after non blanks\n",
		"  -t, --tabs=N     have tabs N characters apart, not 8\n",
		"      --version    output version information and exit\n",
	} {
		if !strings.Contains(helpResult.Stdout, want) {
			t.Fatalf("help stdout = %q, want substring %q", helpResult.Stdout, want)
		}
	}
}

func TestExpandTransformsFilesAndStdin(t *testing.T) {
	t.Parallel()

	runTransformChecks(t, "/tmp/in.txt", []byte("a\tb\n\tlead\ttrail\n"), []transformCheck{
		{name: "file", script: "expand --tabs=4 /tmp/in.txt\n", want: "a   b\n    lead    trail\n"},
		{name: "initial", script: "printf '\\ta\\tb' | expand -i --tabs=4\n", want: "    a\tb"},
		{name: "stdin", script: "printf 'x\\ty' | expand --tabs=4 -\n", want: "x   y"},
		{name: "repeatedStdin", script: "printf 'x\\ty' | expand --tabs=4 - -\n", want: "x   y"},
	})
}

func TestExpandTabListsAndShortcuts(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"printf 'a\\tb\\tc\\td\\te' | expand --tabs '3 6 9'\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\ta\\tb\\tc' | expand --tabs=1,/5\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\ta\\tb\\tc' | expand --tabs=1,+5\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\ta\\tb\\tc' | expand --tabs=8,/4\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\ta\\tb\\tc' | expand --tabs=8,+4\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\ta\\tb\\tc' | expand -2,5 -7\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\ta\\tb\\tc' | expand -8,/4\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\ta\\tb\\tc' | expand -1,+5\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	want := "" +
		"a  b  c  d e---\n" +
		" a   b    c---\n" +
		" a    b    c---\n" +
		"        a   b   c---\n" +
		"        a   b   c---\n" +
		"  a  b c---\n" +
		"        a   b   c---\n" +
		" a    b    c"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestExpandHandlesHugeTabstopWithoutPanicking(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t, &Config{})
	maxInt := strconv.FormatUint(uint64(^uint(0)>>1), 10)
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf '' | expand --tabs=" + maxInt + "\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "" {
		t.Fatalf("Stdout = %q, want empty", result.Stdout)
	}
}

func TestExpandErrorsMatchCoreutilsStyle(t *testing.T) {
	t.Parallel()

	assertTransformErrorScenarios(
		t,
		"expand",
		[]byte("a\tb\n"),
		[]exactStderrCase{
			{name: "invalidFlag", script: "expand -h\n", wantCode: 1, wantStderr: "expand: invalid option -- 'h'\nTry 'expand --help' for more information.\n"},
			{name: "invalidTabs", script: "expand --tabs=1,+2,3\n", wantCode: 1, wantStderr: "expand: '+' specifier only allowed with the last value\n"},
			{name: "invalidLegacyTabs", script: "expand -+5\n", wantCode: 1, wantStderr: "expand: invalid option -- '+'\nTry 'expand --help' for more information.\n"},
			{name: "overflowTabs", script: "expand --tabs=18446744073709551616\n", wantCode: 1, wantStderr: "expand: tab stop is too large '18446744073709551616'\n"},
		},
		"mkdir /tmp/dir\nexpand --tabs=4 /tmp/ok.txt /tmp/dir /tmp/missing.txt\n",
		"a   b\n",
		[]string{
			"expand: /tmp/dir: Is a directory\n",
			"expand: /tmp/missing.txt: No such file or directory\n",
		},
	)
}
