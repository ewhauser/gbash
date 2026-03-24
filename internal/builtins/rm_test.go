package builtins

import (
	"bytes"
	"strings"
	"testing"
)

func parseRMSpec(t *testing.T, args ...string) (*Invocation, *ParsedCommand, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	inv := &Invocation{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	spec := NewRM().Spec()
	parsed, action, err := ParseCommandSpec(inv, &spec)
	return inv, parsed, action, err
}

func TestParseRMSpecParsesGroupedAndOptionalInteractiveFlags(t *testing.T) {
	t.Parallel()

	_, matches, action, err := parseRMSpec(t, "-rfv", "--interactive=once", "target")
	if err != nil {
		t.Fatalf("ParseCommandSpec() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	for _, name := range []string{"force", "recursive", "verbose", "interactive"} {
		if !matches.Has(name) {
			t.Fatalf("%s option not parsed: %#v", name, matches.OptionOrder())
		}
	}
	if got, want := matches.Value("interactive"), "once"; got != want {
		t.Fatalf("interactive value = %q, want %q", got, want)
	}
}

func TestParseRMSpecTreatsBareInteractiveAsAlways(t *testing.T) {
	t.Parallel()

	inv, matches, action, err := parseRMSpec(t, "--interactive", "target")
	if err != nil {
		t.Fatalf("ParseCommandSpec() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	opts, err := parseRMMatches(inv, matches)
	if err != nil {
		t.Fatalf("parseRMMatches() error = %v", err)
	}
	if got, want := opts.interactive, rmInteractiveAlways; got != want {
		t.Fatalf("interactive = %v, want %v", got, want)
	}
}

func TestParseRMSpecRejectsAbbreviatedNoPreserveRoot(t *testing.T) {
	t.Parallel()

	inv, matches, action, err := parseRMSpec(t, "-r", "--no-preserve-r", "/tmp/data")
	if err != nil {
		t.Fatalf("ParseCommandSpec() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	_, err = parseRMMatches(inv, matches)
	if err == nil {
		t.Fatal("parseRMMatches() error = nil, want abbreviation failure")
	}
	if !strings.Contains(err.Error(), "may not abbreviate") {
		t.Fatalf("parseRMMatches() error = %v, want abbreviation diagnostic", err)
	}
}

func TestParseRMSpecAcceptsTripleHyphenPresumeInputTTY(t *testing.T) {
	t.Parallel()

	_, matches, action, err := parseRMSpec(t, "---presume-input-tty", "target")
	if err != nil {
		t.Fatalf("ParseCommandSpec() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	if !matches.Has("presume-input-tty") {
		t.Fatalf("presume-input-tty option not parsed")
	}
}
