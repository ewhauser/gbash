package builtins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/ewhauser/gbash/internal/printfutil"
)

type Printf struct{}

func NewPrintf() *Printf {
	return &Printf{}
}

func (c *Printf) Name() string {
	return "printf"
}

func (c *Printf) Run(ctx context.Context, inv *Invocation) error {
	_ = ctx
	rawArgs := []string(nil)
	if inv != nil {
		rawArgs = inv.Args
	}
	args, err := normalizeGNUPrintfArgs(rawArgs)
	if err != nil {
		return exitf(inv, 1, "printf: %s\nTry 'printf --help' for more information.", err)
	}
	stdout := io.Discard
	stderr := io.Discard
	if inv != nil && inv.Stdout != nil {
		stdout = inv.Stdout
	}
	if inv != nil && inv.Stderr != nil {
		stderr = inv.Stderr
	}
	result := printfutil.Format(args[0], args[1:], printfutil.Options{
		Dialect: printfutil.DialectGNU,
		LookupEnv: func(name string) (string, bool) {
			if inv == nil || inv.Env == nil {
				return "", false
			}
			value, ok := inv.Env[name]
			return value, ok
		},
	})
	for _, diag := range result.Diagnostics {
		_, _ = fmt.Fprintf(stderr, "printf: %s\n", diag)
	}
	for _, warning := range result.Warnings {
		_, _ = fmt.Fprintf(stderr, "printf: %s\n", warning)
	}
	if _, err := io.WriteString(stdout, result.Output); err != nil {
		if printfBrokenPipe(err) {
			if result.ExitCode != 0 {
				return &ExitError{Code: int(result.ExitCode)}
			}
			return nil
		}
		if diag, ok := shellWriteErrorDiagnostic("printf", err); ok {
			return exitf(inv, 1, "%s", diag)
		}
		return &ExitError{Code: 1, Err: err}
	}
	if result.ExitCode != 0 {
		return &ExitError{Code: int(result.ExitCode)}
	}
	return nil
}

func printfBrokenPipe(err error) bool {
	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "broken pipe") || strings.Contains(lower, "closed pipe")
}

func normalizeGNUPrintfArgs(args []string) (normalized []string, err error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("missing operand")
	}
	if args[0] == "--" {
		if len(args) == 1 {
			return nil, fmt.Errorf("missing operand")
		}
		return args[1:], nil
	}
	return args, nil
}

var _ Command = (*Printf)(nil)
