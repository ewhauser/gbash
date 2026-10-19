package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/internal/builtins"
)

func TestPanickingCommandOutsideForegroundFailsWithoutCrashing(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "pipeline left side",
			script: "boom | cat\necho \"after=${PIPESTATUS[*]}\"\n",
			want:   "after=2 0\n",
		},
		{
			name:   "background job",
			script: "boom &\nwait $!\necho after=$?\n",
			want:   "after=2\n",
		},
		{
			name:   "process substitution",
			script: "cat <(boom)\necho after=$?\n",
			want:   "after=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registry := builtins.DefaultRegistry()
			if err := registry.Register(commands.DefineCommand("boom", func(context.Context, *commands.Invocation) error {
				panic("boom: internal invariant violated")
			})); err != nil {
				t.Fatalf("Register(boom) error = %v", err)
			}

			result, err := newRuntime(t, &Config{Registry: registry}).Run(context.Background(), &ExecutionRequest{Script: tc.script})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := result.Stdout; got != tc.want {
				t.Fatalf("Stdout = %q, want %q; stderr=%q", got, tc.want, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "shell execution failed") {
				t.Fatalf("Stderr = %q, want sanitized panic message", result.Stderr)
			}
			if strings.Contains(result.Stderr, "invariant") {
				t.Fatalf("Stderr = %q, leaked panic value", result.Stderr)
			}
		})
	}
}
