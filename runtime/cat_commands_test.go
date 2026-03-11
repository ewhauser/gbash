package runtime

import (
	"context"
	"testing"
)

func TestCatSupportsLongAndShortNumberFlags(t *testing.T) {
	rt := newRuntime(t, &Config{})

	result, err := rt.Run(context.Background(), &ExecutionRequest{
		Script: "printf 'alpha\\nbeta\\n' > /tmp/a.txt\nprintf 'gamma\\n' > /tmp/b.txt\ncat --number /tmp/a.txt /tmp/b.txt\ncat -n /tmp/b.txt\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "     1\talpha\n     2\tbeta\n     3\tgamma\n     1\tgamma\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
