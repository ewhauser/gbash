package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
)

type Cat struct{}

func NewCat() *Cat {
	return &Cat{}
}

func (c *Cat) Name() string {
	return "cat"
}

func (c *Cat) Run(ctx context.Context, inv *Invocation) error {
	number, names, err := parseCatArgs(inv)
	if err != nil {
		return err
	}

	if !number {
		inputs, err := readNamedInputs(ctx, inv, names, true)
		if err != nil {
			return err
		}
		for _, input := range inputs {
			if _, err := inv.Stdout.Write(input.Data); err != nil {
				return &ExitError{Code: 1, Err: err}
			}
		}
		return nil
	}

	inputs, err := readNamedInputs(ctx, inv, names, true)
	if err != nil {
		return err
	}
	lineNumber := 1
	for _, input := range inputs {
		next, err := writeNumberedCat(inv, input.Data, lineNumber)
		if err != nil {
			return err
		}
		lineNumber = next
	}
	return nil
}

func parseCatArgs(inv *Invocation) (number bool, names []string, err error) {
	args := inv.Args
	for len(args) > 0 {
		arg := args[0]
		if arg == "--" {
			return number, args[1:], nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		switch arg {
		case "-n", "--number":
			number = true
		default:
			return false, nil, exitf(inv, 1, "cat: unsupported flag %s", arg)
		}
		args = args[1:]
	}
	return number, args, nil
}

func writeNumberedCat(inv *Invocation, data []byte, start int) (int, error) {
	segments := bytes.Split(data, []byte{'\n'})
	trailingNewline := len(data) > 0 && data[len(data)-1] == '\n'
	lineNumber := start
	for i, segment := range segments {
		if trailingNewline && i == len(segments)-1 && len(segment) == 0 {
			break
		}
		if _, err := fmt.Fprintf(inv.Stdout, "%6d\t", lineNumber); err != nil {
			return lineNumber, &ExitError{Code: 1, Err: err}
		}
		if _, err := inv.Stdout.Write(segment); err != nil {
			return lineNumber, &ExitError{Code: 1, Err: err}
		}
		if i < len(segments)-1 || trailingNewline {
			if _, err := io.WriteString(inv.Stdout, "\n"); err != nil {
				return lineNumber, &ExitError{Code: 1, Err: err}
			}
		}
		lineNumber++
	}
	return lineNumber, nil
}

var _ Command = (*Cat)(nil)
