package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/policy"
)

const braceLimitMessage = "brace expansion: too many words (limit "

func TestMaxBraceExpansionWordsDefaultStopsHugeRange(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	// Without a cap this allocates roughly 16 GB before echo ever runs.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo {1..30000000} | wc -c\necho {1..30000000}\necho unreachable\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if want := braceLimitMessage + "100000), increase policy.Limits.MaxBraceExpansionWords"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestMaxBraceExpansionWordsDefaultAllowsModerateRange(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "set -- {1..100000}\necho $# ${100000}\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "100000 100000\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestMaxBraceExpansionWordsOverride(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{
		LimitOverrides: policy.Limits{MaxBraceExpansionWords: 5},
	})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo {1..5}\necho {a,b}{1..3}\necho unreachable\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "1 2 3 4 5\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if want := braceLimitMessage + "5)"; !strings.Contains(result.Stderr, want) {
		t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
	}
}

func TestMaxBraceExpansionWordsCountsWholeCommand(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxBraceExpansionWords: 10})

	// Each word fits on its own, but together they exceed the cap.
	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo {1..6} {1..6}\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 126 {
		t.Fatalf("ExitCode = %d, want 126; stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stdout != "" {
		t.Fatalf("Stdout = %q, want empty", result.Stdout)
	}
}

func TestMaxBraceExpansionWordsCountsWholeAssignment(t *testing.T) {
	t.Parallel()

	for _, script := range []string{
		"a=({1..6} {1..6})\n",
		"declare -a a=({1..6} {1..6})\n",
		"declare {a,b,c}{1..2} {d,e,f}{1..2}\n",
	} {
		t.Run(script, func(t *testing.T) {
			t.Parallel()
			rt := newRuntimeWithLimits(t, policy.Limits{MaxBraceExpansionWords: 10})
			result, err := rt.Run(context.Background(), &ExecutionRequest{
				Script: script + "echo unreachable\n",
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.ExitCode != 126 {
				t.Fatalf("ExitCode = %d, want 126; stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
			}
			if !strings.Contains(result.Stderr, braceLimitMessage+"10)") {
				t.Fatalf("Stderr = %q, want brace-limit message", result.Stderr)
			}
		})
	}
}

func TestMaxBraceExpansionWordsResetsPerCommand(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxBraceExpansionWords: 10})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "for i in {1..10}; do echo {1..10} >/dev/null; done\na=({1..10}); echo {1..10} | wc -w; echo ${#a[@]}\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "10\n10\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestMaxBraceExpansionWordsInSubshellAndCommandSubstitution(t *testing.T) {
	t.Parallel()
	rt := newRuntimeWithLimits(t, policy.Limits{MaxBraceExpansionWords: 10})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "( echo {1..11} ); echo sub=$?\nx=$(echo {1..11}); echo cmd=$? x=$x\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "sub=126\ncmd=126 x=\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got := strings.Count(result.Stderr, braceLimitMessage+"10)"); got != 2 {
		t.Fatalf("Stderr = %q, want two brace-limit messages", result.Stderr)
	}
}

func TestBraceSequenceNearIntBoundsTerminates(t *testing.T) {
	t.Parallel()
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "echo {9223372036854775806..9223372036854775807}\n" +
			"echo {-9223372036854775807..-9223372036854775808}\n" +
			"echo {1..9223372036854775807..4611686018427387904}\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	want := "9223372036854775806 9223372036854775807\n" +
		"-9223372036854775807 -9223372036854775808\n" +
		"1 4611686018427387905\n"
	if result.Stdout != want {
		t.Fatalf("Stdout = %q, want %q", result.Stdout, want)
	}
}
