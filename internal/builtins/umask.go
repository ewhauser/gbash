package builtins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type Umask struct{}

func NewUmask() *Umask {
	return &Umask{}
}

func (c *Umask) Name() string {
	return "umask"
}

const umaskEnvKey = "GBASH_UMASK"

func (c *Umask) Run(_ context.Context, inv *Invocation) error {
	args := inv.Args

	switch len(args) {
	case 0:
		mask := umaskValue(inv)
		_, err := fmt.Fprintf(inv.Stdout, "%04o\n", mask)
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		return nil
	case 1:
		arg := args[0]
		if strings.HasPrefix(arg, "-") {
			return exitf(inv, 2, "umask: unsupported option %q", arg)
		}
		value, err := strconv.ParseUint(arg, 8, 32)
		if err != nil || value > 0o777 {
			return exitf(inv, 1, "umask: %s: invalid mode", arg)
		}
		inv.Env[umaskEnvKey] = fmt.Sprintf("%04o", value)
		return nil
	default:
		return exitf(inv, 2, "usage: umask [mode]")
	}
}

func umaskValue(inv *Invocation) uint32 {
	if inv == nil {
		return 0o022
	}
	raw := strings.TrimSpace(inv.Env[umaskEnvKey])
	if raw == "" {
		return 0o022
	}
	value, err := strconv.ParseUint(raw, 8, 32)
	if err != nil || value > 0o777 {
		return 0o022
	}
	return uint32(value)
}

var _ Command = (*Umask)(nil)
