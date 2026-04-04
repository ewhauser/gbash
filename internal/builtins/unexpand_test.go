package builtins_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestUnexpandHelpAndVersion(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t, &Config{})

	helpResult, err := rt.Run(context.Background(), &ExecutionRequest{Script: "unexpand --help --tabs=x\n"})
	if err != nil {
		t.Fatalf("Run(help) error = %v", err)
	}
	if helpResult.ExitCode != 0 {
		t.Fatalf("help ExitCode = %d, want 0; stderr=%q", helpResult.ExitCode, helpResult.Stderr)
	}
	for _, want := range []string{
		"Usage: unexpand [OPTION]... [FILE]...\n",
		"  -a, --all         convert all blanks, instead of just initial blanks\n",
		"      --first-only  convert only leading sequences of blanks (overrides -a)\n",
		"  -t, --tabs=N      have tabs N characters apart instead of 8 (enables -a)\n",
		"      --version     output version information and exit\n",
	} {
		if !strings.Contains(helpResult.Stdout, want) {
			t.Fatalf("help stdout = %q, want substring %q", helpResult.Stdout, want)
		}
	}

	versionResult, err := rt.Run(context.Background(), &ExecutionRequest{Script: "unexpand --version --tabs=x\n"})
	if err != nil {
		t.Fatalf("Run(version) error = %v", err)
	}
	if versionResult.ExitCode != 0 {
		t.Fatalf("version ExitCode = %d, want 0; stderr=%q", versionResult.ExitCode, versionResult.Stderr)
	}
	if got, want := versionResult.Stdout, "unexpand (gbash)\n"; got != want {
		t.Fatalf("version stdout = %q, want %q", got, want)
	}
}

func TestUnexpandTransformsFilesAndStdin(t *testing.T) {
	t.Parallel()

	runTransformChecks(t, "/tmp/in.txt", []byte("        A     B\n123 \t1\n"), []transformCheck{
		{name: "file", script: "unexpand --tabs=3 /tmp/in.txt\n", want: "\t\t  A\t  B\n123\t1\n"},
		{name: "firstOnly", script: "printf '        A     B' | unexpand -3\n", want: "\t\t  A     B"},
		{name: "stdin", script: "printf 'a  b  c' | unexpand -a -3 -\n", want: "a\tb\tc"},
		{name: "repeatedStdin", script: "printf '        A' | unexpand -3 - -\n", want: "\t\t  A"},
	})
}

func TestUnexpandTabListsAndGNUCases(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "" +
			"printf '  2\\n' | unexpand --tabs '2 4'\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\t      ' | unexpand -t '3,+6'\n" +
			"printf '%s\\n' ---\n" +
			"printf '\\t      ' | unexpand -t '3,/9'\n" +
			"printf '%s\\n' ---\n" +
			"printf '          ' | unexpand -t '3,+0'\n" +
			"printf '%s\\n' ---\n" +
			"printf '          ' | unexpand -t '3,/0'\n" +
			"printf '%s\\n' ---\n" +
			"printf '1ΔΔΔ5   99999\\n' | unexpand -a\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}

	want := "" +
		"\t2\n" +
		"---\n" +
		"\t\t" +
		"---\n" +
		"\t\t" +
		"---\n" +
		"\t\t\t " +
		"---\n" +
		"\t\t\t " +
		"---\n" +
		"1ΔΔΔ5   99999\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestUnexpandHandlesHugeTabstopWithoutPanicking(t *testing.T) {
	t.Parallel()

	rt := newRuntime(t, &Config{})
	maxInt := strconv.FormatUint(uint64(^uint(0)>>1), 10)
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf '' | unexpand --tabs=" + maxInt + "\n",
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

func TestUnexpandErrorsMatchCoreutilsStyle(t *testing.T) {
	t.Parallel()

	assertTransformErrorScenarios(
		t,
		"unexpand",
		[]byte("        a\n"),
		[]exactStderrCase{
			{name: "invalidFlag", script: "unexpand -f\n", wantCode: 1, wantStderr: "unexpand: invalid option -- 'f'\nTry 'unexpand --help' for more information.\n"},
			{name: "invalidTabs", script: "unexpand --tabs=1,+2,3\n", wantCode: 1, wantStderr: "unexpand: '+' specifier only allowed with the last value\n"},
			{name: "invalidChar", script: "unexpand --tabs=x\n", wantCode: 1, wantStderr: "unexpand: tab size contains invalid character(s): 'x'\n"},
			{name: "overflowTabs", script: "unexpand --tabs=18446744073709551616\n", wantCode: 1, wantStderr: "unexpand: tab stop value is too large\n"},
		},
		"mkdir /tmp/dir\nunexpand /tmp/ok.txt /tmp/dir /tmp/missing.txt\n",
		"\ta\n",
		[]string{
			"unexpand: /tmp/dir: Is a directory\n",
			"unexpand: /tmp/missing.txt: No such file or directory\n",
		},
	)
}
