package commands

import (
	"context"
	"fmt"
	"strings"
)

type Rev struct{}

func NewRev() *Rev {
	return &Rev{}
}

func (c *Rev) Name() string {
	return "rev"
}

func (c *Rev) Run(ctx context.Context, inv *Invocation) error {
	args := inv.Args
	if len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" && args[0] != "--" {
		return exitf(inv, 1, "rev: unsupported flag %s", args[0])
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	inputs, err := readNamedInputs(ctx, inv, args, true)
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

func reverseRunes(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

var _ Command = (*Rev)(nil)
