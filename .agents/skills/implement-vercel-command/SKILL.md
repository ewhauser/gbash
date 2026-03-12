---
name: implement-vercel-command
description: >
  Implement a command from the upstream vercel-labs/just-bash TypeScript repo in this Go port,
  achieving full parity with the upstream implementation. Use this skill whenever the user wants
  to port a command from Vercel's just-bash, implement a missing command from the TODO.md parity
  list, add a new command that exists upstream, or close a gap in command parity. Also trigger
  when the user says "implement", "port", or "add" followed by a command name from the missing
  commands list, or references "command parity", "upstream command", or "vercel command".
---

# Implement Vercel Command

Port a command from the upstream [vercel-labs/just-bash](https://github.com/vercel-labs/just-bash)
TypeScript repo into this Go codebase with full parity — every flag, every behavior, no shortcuts.

## Workflow

### 1. Pick the command

Read `TODO.md` and present the **Command Parity** section to the user. The missing commands and
missing flags are listed there. Ask the user which command they want to implement. If they already
told you, skip the asking and proceed.

### 2. Clone upstream and study the TypeScript source

```bash
UPSTREAM=$(mktemp -d)/just-bash
GIT_LFS_SKIP_SMUDGE=1 git clone --depth 1 https://github.com/vercel-labs/just-bash.git "$UPSTREAM"
```

Find the command's TypeScript implementation. The upstream structure is:
- `src/commands/<name>.ts` — the command implementation
- `src/commands/registry.ts` — command registration
- Help objects (usually `<name>Help` in the same file) define flags and usage

Read the **entire** TypeScript file. Do not skim. You need to understand every flag, every edge
case, every error path. Pay special attention to:
- The help object's `options` array — this is the authoritative list of flags
- Default values and flag aliases (short + long forms)
- How stdin is handled when no file arguments are given
- Error messages and exit codes
- Any special formatting or output behavior

### 3. Plan the implementation

Before writing code, list out:
- Every flag the command supports (short form, long form, description, default value)
- The stdin behavior (does it read from stdin when no files given?)
- Error conditions and their exit codes
- Any edge cases visible in the TypeScript source

Present this plan to the user for confirmation. This is a checkpoint — the user should agree
the scope is correct before you start coding.

### 4. Implement the command

Read the bundled reference files in this skill's `references/` directory — they contain
everything you need without having to search the codebase:
- `references/helpers.md` — all available helper functions (filesystem, IO, text, errors)
- `references/test-template.md` — test helpers and boilerplate
- `references/fuzz-template.md` — fuzz test structure, oracle selection, Makefile integration

Create `commands/<name>.go` following these patterns:

```go
package commands

import (
    "context"
    // other imports as needed
)

type <Name> struct{}

func New<Name>() *<Name> { return &<Name>{} }

func (c *<Name>) Name() string { return "<name>" }

func (c *<Name>) Run(ctx context.Context, inv *Invocation) error {
    // 1. Parse flags manually from inv.Args (no external flag library)
    // 2. Read inputs using readNamedInputs() or readAllFile()/readAllStdin()
    // 3. Process data
    // 4. Write output to inv.Stdout
    return nil
}

var _ Command = (*<Name>)(nil)
```

Key conventions:
- Use `exitf(inv, code, format, args...)` for error messages to stderr
- Use `allowPath()`, `openRead()`, `readAllFile()`, `readNamedInputs()` for filesystem access
  (see `references/helpers.md` for the full list — don't reimplement these)
- Read from `inv.Stdin` when no file arguments are given (if the command is a filter).
  For multi-file commands, use `readNamedInputs(ctx, inv, names, true)` which handles
  stdin fallback and `-` as stdin automatically.
- For flags with values, support both `-f value` and `-fvalue` (attached) and `--flag=value`
  forms. See existing commands like `cut.go` or `wc.go` for examples.

**Full parity means full parity.** Implement every flag from the upstream help object. Do not
skip flags because they seem obscure or rarely used. Do not leave TODO comments for "later".
If the upstream command supports `-t`, `-s`, `-o`, `-c`, and `-n`, your Go version supports
all five. Compare your implementation against the TypeScript source line by line to make sure
nothing was missed.

### 5. Register the command

Add `New<Name>()` to `DefaultRegistry()` in `commands/registry.go`. Place it in the
appropriate category group, keeping alphabetical order within the group.

### 6. Write tests

See `references/test-template.md` for the full template and helpers. Add integration tests
in the appropriate `runtime/*_commands_test.go` file.

Cover:
- **Every flag** — at least one test per flag, testing it actually works
- **Flag combinations** — common combinations that users would use together
- **Happy path** — basic invocation with typical input
- **Error cases** — missing required args, nonexistent files, invalid flag values
- **Stdin input** — pipe behavior when the command supports it
- **Edge cases** — empty input, binary data, large input, special characters
- **Exit codes** — verify correct exit codes for both success and failure

The test coverage should be thorough enough that you could delete the TypeScript source and
reconstruct the command's behavior entirely from the tests.

### 7. Write fuzz tests

This is mandatory — the sandbox is a security boundary. See `references/fuzz-template.md`
for the complete template, helper list, and oracle selection guide.

Add fuzz coverage in `runtime/fuzz_command_targets_test.go` (or extend an existing fuzz
function if the command fits an existing category — the template lists all existing categories).

Key points:
- Use `newFuzzRuntime`, `newFuzzSession`, `runFuzzSessionScript`
- Provide 2-3 seed inputs with `f.Add()`
- **Use `assertSecureFuzzOutcome` as the oracle** — this is the correct default for all new
  commands. It allows non-zero exits but catches crashes, host path leaks, and sensitive
  disclosure. Do NOT use `assertBaseFuzzOutcome` or `assertSuccessfulFuzzExecution` unless
  you have a specific reason.
- Test the command with various flag combinations and malformed input
- Add the fuzz target to the Makefile

### 8. Update SPEC.md

Add the new command to the appropriate section in `SPEC.md`.

### 9. Update TODO.md

Check off the command in the `## Command Parity` section of `TODO.md`.

### 10. Verify everything builds and passes

```bash
go build ./...
go test ./...
make fuzz FUZZTIME=10s
make lint
```

Fix any failures before presenting the result to the user. All four commands must pass clean.

## Important reminders

- **Do not skip flags.** The whole point of this skill is achieving full parity. If you're
  tempted to skip a flag, don't. Implement it.
- **Read the TypeScript source carefully.** Subtle behaviors hide in conditionals and default
  values. A quick skim will miss them.
- **Test what you implement.** Every flag needs at least one test proving it works. Untested
  code is unfinished code.
- **Fuzz tests are not optional.** The sandbox is a security boundary. Every command that
  processes input needs fuzz coverage.
- **Check your work.** After implementation, go back to the TypeScript source and verify
  every flag and behavior is accounted for in your Go code.
