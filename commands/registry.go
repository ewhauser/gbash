package commands

import (
	"context"
	"slices"
	"sync"
)

// LazyCommandLoader constructs a command on first use.
type LazyCommandLoader func() (Command, error)

// CommandRegistry is the interface expected by gbash runtimes and embedders
// that need command lookup and registration.
type CommandRegistry interface {
	Register(cmd Command) error
	RegisterLazy(name string, loader LazyCommandLoader) error
	Lookup(name string) (Command, bool)
	Names() []string
}

// Registry is the default in-memory [CommandRegistry] implementation.
type Registry struct {
	mu       sync.RWMutex
	commands map[string]Command
}

type lazyCommand struct {
	name   string
	loader LazyCommandLoader
	once   sync.Once
	cmd    Command
	err    error
}

// NewRegistry constructs a registry and eagerly registers cmds.
func NewRegistry(cmds ...Command) *Registry {
	registry := &Registry{
		commands: make(map[string]Command),
	}
	for _, cmd := range cmds {
		_ = registry.Register(cmd)
	}
	return registry
}

// Register stores cmd by name, replacing any existing command with the same
// name.
func (r *Registry) Register(cmd Command) error {
	if cmd == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.commands[cmd.Name()] = cmd
	return nil
}

// RegisterLazy registers a name that will be materialized by loader on first
// execution.
func (r *Registry) RegisterLazy(name string, loader LazyCommandLoader) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.commands[name] = &lazyCommand{
		name:   name,
		loader: loader,
	}
	return nil
}

// Lookup returns the registered command for name.
func (r *Registry) Lookup(name string) (Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.commands[name]
	return cmd, ok
}

// Names returns the sorted list of registered command names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (c *lazyCommand) Name() string {
	return c.name
}

func (c *lazyCommand) Run(ctx context.Context, inv *Invocation) error {
	cmd, err := c.load()
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	return RunCommand(ctx, cmd, inv)
}

func (c *lazyCommand) load() (Command, error) {
	c.once.Do(func() {
		if c.loader == nil {
			c.err = nil
			c.cmd = DefineCommand(c.name, nil)
			return
		}
		cmd, err := c.loader()
		if err != nil {
			c.err = err
			return
		}
		if cmd == nil {
			c.err = &ExitError{Code: 1}
			return
		}
		c.cmd = cmd
	})
	if c.err != nil {
		return nil, c.err
	}
	return c.cmd, nil
}

var _ CommandRegistry = (*Registry)(nil)
