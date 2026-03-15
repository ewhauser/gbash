package builtins

import (
	"context"
	"fmt"

	"github.com/ewhauser/gbash/internal/shellstate"
)

type Complete struct{}

type completeConfig struct {
	printMode   bool
	removeMode  bool
	isDefault   bool
	wordlist    string
	hasWordlist bool
	function    string
	hasFunction bool
	command     string
	hasCommand  bool
	options     []string
	actions     []string
	commands    []string
}

func NewComplete() *Complete {
	return &Complete{}
}

func (c *Complete) Name() string {
	return "complete"
}

func (c *Complete) Run(ctx context.Context, inv *Invocation) error {
	state := completionStateFromContext(ctx)
	cfg, err := parseCompleteArgs(inv.Args)
	if err != nil {
		return exitf(inv, 2, err.Error())
	}

	if cfg.removeMode {
		if len(cfg.commands) == 0 {
			state.Clear()
			return nil
		}
		for _, name := range cfg.commands {
			state.Delete(name)
		}
		return nil
	}

	if cfg.printMode {
		return printCompletionSpecs(inv, state, cfg.commands)
	}

	if len(inv.Args) == 0 || (len(cfg.commands) == 0 &&
		!cfg.hasWordlist &&
		!cfg.hasFunction &&
		!cfg.hasCommand &&
		len(cfg.options) == 0 &&
		len(cfg.actions) == 0 &&
		!cfg.isDefault) {
		return printCompletionSpecs(inv, state, nil)
	}

	if cfg.hasFunction && len(cfg.commands) == 0 && !cfg.isDefault {
		return exitf(inv, 2, "complete: -F: option requires a command name")
	}

	if cfg.isDefault {
		spec := completionSpecFromConfig(cfg, true)
		state.Set(shellstate.CompletionSpecDefaultKey, &spec)
		return nil
	}

	for _, name := range cfg.commands {
		spec := completionSpecFromConfig(cfg, false)
		state.Set(name, &spec)
	}
	return nil
}

func parseCompleteArgs(args []string) (*completeConfig, error) {
	cfg := &completeConfig{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-p":
			cfg.printMode = true
		case "-r":
			cfg.removeMode = true
		case "-D":
			cfg.isDefault = true
		case "-W":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("complete: -W: option requires an argument")
			}
			cfg.wordlist = args[i]
			cfg.hasWordlist = true
		case "-F":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("complete: -F: option requires an argument")
			}
			cfg.function = args[i]
			cfg.hasFunction = true
		case "-o":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("complete: -o: option requires an argument")
			}
			opt := args[i]
			if !isValidCompletionOption(opt) {
				return nil, fmt.Errorf("complete: %s: invalid option name", opt)
			}
			cfg.options = append(cfg.options, opt)
		case "-A":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("complete: -A: option requires an argument")
			}
			cfg.actions = append(cfg.actions, args[i])
		case "-C":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("complete: -C: option requires an argument")
			}
			cfg.command = args[i]
			cfg.hasCommand = true
		case "-G", "-P", "-S", "-X":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("complete: %s: option requires an argument", arg)
			}
		case "--":
			cfg.commands = append(cfg.commands, args[i+1:]...)
			return cfg, nil
		default:
			if arg != "" && arg[0] != '-' {
				cfg.commands = append(cfg.commands, arg)
			}
		}
	}
	return cfg, nil
}

func completionSpecFromConfig(cfg *completeConfig, isDefault bool) shellstate.CompletionSpec {
	spec := shellstate.CompletionSpec{IsDefault: isDefault}
	if cfg == nil {
		return spec
	}
	if cfg.hasWordlist {
		spec.Wordlist = cfg.wordlist
		spec.HasWordlist = true
	}
	if cfg.hasFunction {
		spec.Function = cfg.function
		spec.HasFunction = true
	}
	if cfg.hasCommand {
		spec.Command = cfg.command
		spec.HasCommand = true
	}
	if len(cfg.options) > 0 {
		spec.Options = append(spec.Options, cfg.options...)
	}
	if len(cfg.actions) > 0 {
		spec.Actions = append(spec.Actions, cfg.actions...)
	}
	return spec
}

func printCompletionSpecs(inv *Invocation, state *shellstate.CompletionState, commands []string) error {
	if len(commands) == 0 {
		for _, name := range state.Keys() {
			if isInternalCompletionSpec(name) {
				continue
			}
			spec, ok := state.Get(name)
			if !ok {
				continue
			}
			if _, err := fmt.Fprintln(inv.Stdout, formatCompleteSpec(name, &spec)); err != nil {
				return &ExitError{Code: 1, Err: err}
			}
		}
		return nil
	}

	for _, name := range commands {
		spec, ok := state.Get(name)
		if !ok {
			_, _ = fmt.Fprintf(inv.Stderr, "complete: %s: no completion specification\n", name)
			return &ExitError{Code: 1}
		}
		if _, err := fmt.Fprintln(inv.Stdout, formatCompleteSpec(name, &spec)); err != nil {
			return &ExitError{Code: 1, Err: err}
		}
	}
	return nil
}

var _ Command = (*Complete)(nil)
