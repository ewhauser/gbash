package builtins

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Rev struct{}

func NewRev() *Rev {
	return &Rev{}
}

func (c *Rev) Name() string {
	return "rev"
}

func (c *Rev) Run(ctx context.Context, inv *Invocation) error {
	return RunCommand(ctx, c, inv)
}

func (c *Rev) Spec() CommandSpec {
	return CommandSpec{
		Name:  "rev",
		About: "Reverse lines characterwise",
		Usage: "rev [FILE]...",
		Args: []ArgSpec{
			{Name: "file", ValueName: "FILE", Repeatable: true},
		},
		Parse: ParseConfig{
			StopAtFirstPositional: true,
		},
	}
}

func (c *Rev) RunParsed(ctx context.Context, inv *Invocation, matches *ParsedCommand) error {
	args := matches.Args("file")
	inputs, err := readNamedInputs(ctx, inv, args)
	if err != nil {
		return err
	}
	for _, input := range inputs {
		for _, line := range textLines(input.Data) {
			if _, err := fmt.Fprintln(inv.Stdout, reverseRunes(line)); err != nil {
				return &ExitError{Code: 1, Err: err}
			}
		}
	}
	return nil
}

// reverseRunes reverses s by UTF-8 decode units, keeping each invalid byte
// as its own unit instead of replacing it with U+FFFD.
func reverseRunes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for end := len(s); end > 0; {
		_, size := utf8.DecodeLastRuneInString(s[:end])
		b.WriteString(s[end-size : end])
		end -= size
	}
	return b.String()
}

var _ Command = (*Rev)(nil)
