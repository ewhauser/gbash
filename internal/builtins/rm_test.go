package builtins

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	stdfs "io/fs"

	gbfs "github.com/ewhauser/gbash/fs"
)

type removeNotExistAfterDeleteFS struct {
	gbfs.FileSystem
	target string
}

func (fs removeNotExistAfterDeleteFS) Remove(ctx context.Context, name string, recursive bool) error {
	if name == fs.target {
		if err := fs.FileSystem.Remove(ctx, name, recursive); err != nil {
			return err
		}
		return stdfs.ErrNotExist
	}
	return fs.FileSystem.Remove(ctx, name, recursive)
}

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

func TestParseRMSpecUsesPerOccurrenceInteractiveValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want rmInteractiveMode
	}{
		{
			name: "bare interactive overrides prior value",
			args: []string{"--interactive=never", "--interactive", "target"},
			want: rmInteractiveAlways,
		},
		{
			name: "explicit never remains last override",
			args: []string{"--interactive", "--interactive=never", "target"},
			want: rmInteractiveNever,
		},
		{
			name: "interactive clears prior force",
			args: []string{"-f", "-i", "target"},
			want: rmInteractiveAlways,
		},
		{
			name: "interactive never preserves prior force",
			args: []string{"-f", "--interactive=never", "target"},
			want: rmInteractiveNever,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inv, matches, action, err := parseRMSpec(t, tt.args...)
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
			if got := opts.interactive; got != tt.want {
				t.Fatalf("interactive = %v, want %v", got, tt.want)
			}
			switch tt.name {
			case "interactive clears prior force":
				if opts.force {
					t.Fatalf("force = true, want false after later interactive option")
				}
			case "interactive never preserves prior force":
				if !opts.force {
					t.Fatalf("force = false, want true when --interactive=never follows -f")
				}
			}
		})
	}
}

func TestParseRMSpecRejectsExplicitEmptyInteractiveValue(t *testing.T) {
	t.Parallel()

	inv, matches, action, err := parseRMSpec(t, "--interactive=", "target")
	if err != nil {
		t.Fatalf("ParseCommandSpec() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	_, err = parseRMMatches(inv, matches)
	if err == nil {
		t.Fatal("parseRMMatches() error = nil, want empty interactive value failure")
	}
	if !strings.Contains(err.Error(), "invalid argument '' for '--interactive'") {
		t.Fatalf("parseRMMatches() error = %v, want empty interactive diagnostic", err)
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

func TestParseRMSpecRejectsAbbreviatedNoPreserveRootWithExactPositional(t *testing.T) {
	t.Parallel()

	inv, matches, action, err := parseRMSpec(t, "-r", "--no-preserve-r", "--", "--no-preserve-root")
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

func TestRMForceIgnoresRemoveNotExistAfterSuccessfulLstat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mem := gbfs.NewMemory()
	file, err := mem.OpenFile(ctx, "/tmp/race", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	var stderr bytes.Buffer
	inv := NewInvocation(&InvocationOptions{
		Cwd:        "/",
		FileSystem: removeNotExistAfterDeleteFS{FileSystem: mem, target: "/tmp/race"},
		Stderr:     &stderr,
	})
	info, err := inv.FS.Lstat(ctx, "/tmp/race")
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}

	result, err := rmRemoveFile(ctx, inv, "/tmp/race", "/tmp/race", info, rmOptions{force: true})
	if err != nil {
		t.Fatalf("rmRemoveFile() error = %v", err)
	}
	if result.hadErr {
		t.Fatalf("hadErr = true, want false")
	}
	if !result.removed {
		t.Fatalf("removed = false, want true when target disappears during forced removal")
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if _, err := inv.FS.Lstat(ctx, "/tmp/race"); !errorsIsNotExist(err) {
		t.Fatalf("post-remove Lstat() error = %v, want not exist", err)
	}
}
