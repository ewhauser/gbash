//go:build js

package builtins

func bashHelpPlatform(inv *Invocation) string {
	return archMachine(inv) + "-unknown-js"
}
