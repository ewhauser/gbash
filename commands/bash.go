package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
)

type Bash struct {
	name string
}

func NewBash() *Bash {
	return &Bash{name: "bash"}
}

func NewSh() *Bash {
	return &Bash{name: "sh"}
}

func (c *Bash) Name() string {
	return c.name
}

func (c *Bash) Run(ctx context.Context, inv *Invocation) error {
	return RunCommand(ctx, c, inv)
}

func (c *Bash) Spec() CommandSpec {
	return CommandSpec{
		Name:  c.name,
		Usage: c.name + " [-c command_string [name [arg ...]]] [script [arg ...]]",
		Options: []OptionSpec{
			{Name: "command", Short: 'c', ValueName: "command_string", Arity: OptionRequiredValue, Help: "read commands from command_string"},
			{Name: "stdin", Short: 's', Help: "read commands from standard input"},
		},
		Args: []ArgSpec{
			{Name: "arg", ValueName: "arg", Repeatable: true},
		},
		Parse: ParseConfig{
			StopAtFirstPositional: true,
			AutoHelp:              true,
			AutoVersion:           true,
		},
		HelpRenderer: func(w io.Writer, spec CommandSpec) error {
			_, err := fmt.Fprintf(w, "usage: %s\n", spec.Usage)
			return err
		},
	}
}

func (c *Bash) NormalizeInvocation(inv *Invocation) *Invocation {
	if !c.shouldUseHostShell(inv) || inv == nil || len(inv.Args) == 0 {
		return inv
	}
	switch inv.Args[0] {
	case "--help", "--version":
		clone := *inv
		clone.Args = append([]string{"--"}, inv.Args...)
		return &clone
	default:
		return inv
	}
}

func (c *Bash) RunParsed(ctx context.Context, inv *Invocation, matches *ParsedCommand) error {
	if inv.Exec == nil {
		return fmt.Errorf("%s: subexec callback missing", c.name)
	}
	if c.shouldUseHostShell(inv) {
		return c.runHostShell(ctx, inv)
	}

	if matches.Has("command") {
		positional := matches.Args("arg")
		if len(positional) > 0 {
			positional = positional[1:]
		}
		return c.executeInlineScript(ctx, inv, matches.Value("command"), positional, inv.Stdin)
	}

	if matches.Has("stdin") || len(matches.Args("arg")) == 0 {
		return c.executeStdinScript(ctx, inv, matches.Args("arg"))
	}

	args := matches.Args("arg")
	scriptData, _, err := readAllFile(ctx, inv, args[0])
	if err != nil {
		return exitf(inv, 127, "%s: %s: No such file or directory", c.name, args[0])
	}
	result, err := inv.Exec(ctx, &ExecutionRequest{
		Script:  string(scriptData),
		Args:    args[1:],
		Env:     inv.Env,
		WorkDir: inv.Cwd,
		Stdin:   inv.Stdin,
	})
	if err != nil {
		return err
	}
	if err := writeExecutionOutputs(inv, result); err != nil {
		return err
	}
	return exitForExecutionResult(result)
}

func (c *Bash) executeStdinScript(ctx context.Context, inv *Invocation, positional []string) error {
	data, err := io.ReadAll(inv.Stdin)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if len(data) == 0 {
		return nil
	}
	return c.executeInlineScript(ctx, inv, string(data), positional, nil)
}

func (c *Bash) executeInlineScript(ctx context.Context, inv *Invocation, script string, positional []string, stdin io.Reader) error {
	result, err := inv.Exec(ctx, &ExecutionRequest{
		Script:  script,
		Args:    positional,
		Env:     inv.Env,
		WorkDir: inv.Cwd,
		Stdin:   stdin,
	})
	if err != nil {
		return err
	}
	if err := writeExecutionOutputs(inv, result); err != nil {
		return err
	}
	return exitForExecutionResult(result)
}

func (c *Bash) shouldUseHostShell(inv *Invocation) bool {
	if c.name != "bash" || inv == nil {
		return false
	}
	return inv.Env["GBASH_USE_HOST_BASH"] == "1"
}

func (c *Bash) runHostShell(ctx context.Context, inv *Invocation) error {
	shellPath, err := c.hostShellPath()
	if err != nil {
		return err
	}

	args := append([]string(nil), inv.Args...)
	if len(args) == 0 {
		args = []string{"-s"}
	}

	cmd := osexec.CommandContext(ctx, shellPath, args...)
	cmd.Dir = inv.Cwd
	cmd.Env = sortedEnvPairs(inv.Env)
	cmd.Stdin = inv.Stdin
	cmd.Stdout = inv.Stdout
	cmd.Stderr = inv.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Code: exitErr.ExitCode()}
		}
		return &ExitError{Code: exitCodeForError(err), Err: err}
	}
	return nil
}

func (c *Bash) hostShellPath() (string, error) {
	for _, candidate := range []string{"/bin/bash", "/usr/bin/bash"} {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s: host bash not available", c.name)
}

var _ Command = (*Bash)(nil)
var _ SpecProvider = (*Bash)(nil)
var _ ParsedRunner = (*Bash)(nil)
var _ ParseInvocationNormalizer = (*Bash)(nil)
