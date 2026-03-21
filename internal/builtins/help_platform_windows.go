//go:build windows

package builtins

import "runtime"

func bashHelpPlatform(inv *Invocation) string {
	return archMachine(inv) + "-unknown-" + runtime.GOOS
}
