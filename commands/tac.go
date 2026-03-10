package commands

import (
	"context"
	"fmt"
	"strings"
)

type Tac struct{}

func NewTac() *Tac {
	return &Tac{}
}

func (c *Tac) Name() string {
	return "tac"
}

func (c *Tac) Run(ctx context.Context, inv *Invocation) error {
	args := inv.Args
	if len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" && args[0] != "--" {
		return exitf(inv, 1, "tac: unsupported flag %s", args[0])
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	inputs, err := readNamedInputs(ctx, inv, args, true)
	if err != nil {
		return err
	}
	for _, input := range inputs {
		lines := textLines(input.Data)
		for i := len(lines) - 1; i >= 0; i-- {
			if _, err := fmt.Fprintln(inv.Stdout, lines[i]); err != nil {
				return &ExitError{Code: 1, Err: err}
			}
		}
	}
	return nil
}

var _ Command = (*Tac)(nil)
