package commands

import (
	"context"
	"fmt"
	"strings"
)

type Comm struct{}

type commOptions struct {
	suppress [4]bool
}

func NewComm() *Comm {
	return &Comm{}
}

func (c *Comm) Name() string {
	return "comm"
}

func (c *Comm) Run(ctx context.Context, inv *Invocation) error {
	opts, leftName, rightName, err := parseCommArgs(inv)
	if err != nil {
		return err
	}

	leftData, rightData, err := readTwoInputs(ctx, inv, leftName, rightName)
	if err != nil {
		return err
	}
	left := textLines(leftData)
	right := textLines(rightData)

	i, j := 0, 0
	for i < len(left) || j < len(right) {
		switch {
		case i >= len(left):
			if err := writeCommLine(inv, opts, 2, right[j]); err != nil {
				return err
			}
			j++
		case j >= len(right):
			if err := writeCommLine(inv, opts, 1, left[i]); err != nil {
				return err
			}
			i++
		case left[i] == right[j]:
			if err := writeCommLine(inv, opts, 3, left[i]); err != nil {
				return err
			}
			i++
			j++
		case left[i] < right[j]:
			if err := writeCommLine(inv, opts, 1, left[i]); err != nil {
				return err
			}
			i++
		default:
			if err := writeCommLine(inv, opts, 2, right[j]); err != nil {
				return err
			}
			j++
		}
	}
	return nil
}

func parseCommArgs(inv *Invocation) (opts commOptions, leftName, rightName string, err error) {
	args := inv.Args

	for len(args) > 0 {
		arg := args[0]
		if arg == "--" {
			args = args[1:]
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		for _, flag := range arg[1:] {
			switch flag {
			case '1', '2', '3':
				opts.suppress[flag-'0'] = true
			default:
				return commOptions{}, "", "", exitf(inv, 1, "comm: unsupported flag -%c", flag)
			}
		}
		args = args[1:]
	}

	if len(args) != 2 {
		return commOptions{}, "", "", exitf(inv, 1, "comm: expected exactly two input files")
	}
	return opts, args[0], args[1], nil
}

func writeCommLine(inv *Invocation, opts commOptions, column int, line string) error {
	if opts.suppress[column] {
		return nil
	}
	prefixTabs := 0
	for i := 1; i < column; i++ {
		if !opts.suppress[i] {
			prefixTabs++
		}
	}
	if _, err := fmt.Fprint(inv.Stdout, strings.Repeat("\t", prefixTabs)); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if _, err := fmt.Fprintln(inv.Stdout, line); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return nil
}

var _ Command = (*Comm)(nil)
