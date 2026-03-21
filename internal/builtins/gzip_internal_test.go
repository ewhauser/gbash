package builtins

import (
	"bytes"
	"testing"
)

func parseGzipSpec(t *testing.T, args ...string) (parsed *ParsedCommand, action string, err error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inv := &Invocation{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	spec := NewGzip().Spec()
	return ParseCommandSpec(inv, &spec)
}

func TestParseGzipSpecParsesGroupedShortFlags(t *testing.T) {
	t.Parallel()

	matches, action, err := parseGzipSpec(t, "-cdfq", "--", "file.gz")
	if err != nil {
		t.Fatalf("ParseCommandSpec() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	for _, name := range []string{"stdout", "decompress", "force", "quiet"} {
		if !matches.Has(name) {
			t.Fatalf("%s option not parsed", name)
		}
	}
}
