// Package commands provides the stable command authoring and registry API for
// gbash.
//
// Most embedders should construct and run sandboxes through the root
// `github.com/ewhauser/gbash` package and only import commands when they need
// to:
//
//   - implement custom commands
//   - customize a command registry
//   - reuse gbash's command-spec parsing, bounded-read helpers, and
//     invocation helpers
//
// gbash's shipped command implementations live under internal/ and are not part
// of this package's public API.
//
// The usual extension flow is:
//
//   - define a command with [DefineCommand] or a type that implements [Command]
//   - read stdin or files with [ReadAll], [ReadAllStdin], or [CommandFS.ReadFile]
//   - access the sandboxed filesystem through [Invocation.FS]
//   - use [Invocation.Fetch] and [Invocation.Exec] instead of host networking or
//     subprocess APIs
//   - register commands with [NewRegistry] and pass that registry to the root
//     gbash runtime
//
// A minimal custom command looks like:
//
//	registry := commands.NewRegistry(
//		commands.DefineCommand("upper", func(ctx context.Context, inv *commands.Invocation) error {
//			data, err := commands.ReadAllStdin(ctx, inv)
//			if err != nil {
//				return err
//			}
//			_, err = inv.Stdout.Write(bytes.ToUpper(data))
//			return err
//		}),
//	)
//
// The package is intentionally capability-oriented. Custom commands should
// prefer [Invocation], [CommandFS], and the shared helpers in this package over
// direct use of host APIs so policy enforcement, tracing, and size limits stay
// consistent with built-in commands.
package commands
