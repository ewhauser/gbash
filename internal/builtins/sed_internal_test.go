package builtins

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeSedArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "short in-place suffix",
			args: []string{"-i.bak", "s/x/y/", "file"},
			want: []string{"--in-place=.bak", "s/x/y/", "file"},
		},
		{
			name: "grouped short flags before in-place suffix",
			args: []string{"-Ei.bak", "s/x/y/", "file"},
			want: []string{"-E", "--in-place=.bak", "s/x/y/", "file"},
		},
		{
			name: "plain grouped flags stay grouped",
			args: []string{"-ni", "s/x/y/", "file"},
			want: []string{"-ni", "s/x/y/", "file"},
		},
		{
			name: "script value after expression is not rewritten",
			args: []string{"-e", "-i.bak", "file"},
			want: []string{"-e", "-i.bak", "file"},
		},
		{
			name: "file after script is not rewritten",
			args: []string{"s/x/y/", "-i.bak"},
			want: []string{"s/x/y/", "-i.bak"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSedArgs(tc.args); !slices.Equal(got, tc.want) {
				t.Fatalf("normalizeSedArgs(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestRunSedProgramStopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	program, err := parseSedProgram([]string{"s/x*/yy/g"}, false)
	if err != nil {
		t.Fatalf("parseSedProgram() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	run := &sedRun{ctx: ctx, out: &out, maxPatternBytes: 1 << 20}
	if err := runSedProgram(run, program, []string{"abc", "def"}, false, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("runSedProgram() error = %v, want context.Canceled", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want none after cancellation", out.String())
	}
}

func TestSedReplaceCapsPatternSpace(t *testing.T) {
	t.Parallel()

	program, err := parseSedProgram([]string{"s/x*/0123456789/g"}, false)
	if err != nil {
		t.Fatalf("parseSedProgram() error = %v", err)
	}
	var out bytes.Buffer
	run := &sedRun{ctx: context.Background(), out: &out, maxPatternBytes: 1000}
	if err := runSedProgram(run, program, []string{strings.Repeat("a", 200)}, false, true); !errors.Is(err, errSedPatternSpaceTooLarge) {
		t.Fatalf("runSedProgram() error = %v, want errSedPatternSpaceTooLarge", err)
	}
}

func TestSedOutputBufferHonorsLimit(t *testing.T) {
	t.Parallel()

	buffer := &sedOutputBuffer{limit: 4}
	if _, err := buffer.Write([]byte("abcd")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := buffer.Write([]byte("e")); !errors.Is(err, errSedOutputTooLarge) {
		t.Fatalf("Write() error = %v, want errSedOutputTooLarge", err)
	}
}
