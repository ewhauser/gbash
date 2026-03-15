package builtins

import (
	"context"
	"fmt"

	"github.com/ewhauser/gbash/internal/shellstate"
)

type Compopt struct{}

type compoptConfig struct {
	isDefault      bool
	isEmptyLine    bool
	enableOptions  []string
	disableOptions []string
	commands       []string
}

func NewCompopt() *Compopt {
	return &Compopt{}
}

func (c *Compopt) Name() string {
	return "compopt"
}

func (c *Compopt) Run(ctx context.Context, inv *Invocation) error {
	state := completionStateFromContext(ctx)
	cfg, err := parseCompoptArgs(inv.Args)
	if err != nil {
		return exitf(inv, 2, err.Error())
	}

	switch {
	case cfg.isDefault:
		state.Update(shellstate.CompletionSpecDefaultKey, func(spec *shellstate.CompletionSpec) {
			spec.IsDefault = true
			spec.Options = mergeCompletionOptions(spec.Options, cfg.enableOptions, cfg.disableOptions)
		})
		return nil
	case cfg.isEmptyLine:
		state.Update(shellstate.CompletionSpecEmptyKey, func(spec *shellstate.CompletionSpec) {
			spec.Options = mergeCompletionOptions(spec.Options, cfg.enableOptions, cfg.disableOptions)
		})
		return nil
	case len(cfg.commands) > 0:
		for _, name := range cfg.commands {
			state.Update(name, func(spec *shellstate.CompletionSpec) {
				spec.Options = mergeCompletionOptions(spec.Options, cfg.enableOptions, cfg.disableOptions)
			})
		}
		return nil
	default:
		return exitf(inv, 1, "compopt: not currently executing completion function")
	}
}

func parseCompoptArgs(args []string) (*compoptConfig, error) {
	cfg := &compoptConfig{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-D":
			cfg.isDefault = true
		case "-E":
			cfg.isEmptyLine = true
		case "-o":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("compopt: -o: option requires an argument")
			}
			opt := args[i]
			if !isValidCompletionOption(opt) {
				return nil, fmt.Errorf("compopt: %s: invalid option name", opt)
			}
			cfg.enableOptions = append(cfg.enableOptions, opt)
		case "+o":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("compopt: +o: option requires an argument")
			}
			opt := args[i]
			if !isValidCompletionOption(opt) {
				return nil, fmt.Errorf("compopt: %s: invalid option name", opt)
			}
			cfg.disableOptions = append(cfg.disableOptions, opt)
		case "--":
			cfg.commands = append(cfg.commands, args[i+1:]...)
			return cfg, nil
		default:
			if arg != "" && arg[0] != '-' && arg[0] != '+' {
				cfg.commands = append(cfg.commands, arg)
			}
		}
	}
	return cfg, nil
}

var _ Command = (*Compopt)(nil)
