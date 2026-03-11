---
name: upstream-diff
description: >
  Diff this repo against the upstream vercel-labs/just-bash TypeScript repo to find
  missing commands and missing flags. Use this skill whenever the user asks about
  upstream parity, porting commands from just-bash, checking what's missing compared
  to Vercel's version, syncing with upstream, or updating the command parity TODO list.
  Also use it when the user says "upstream diff", "what commands are we missing",
  "sync with just-bash", or "update parity".
---

# Upstream Diff

Compare commands and flags between this Go port and the upstream
[vercel-labs/just-bash](https://github.com/vercel-labs/just-bash) TypeScript repo.

## What it does

1. Clones (or refreshes) the upstream `vercel-labs/just-bash` repo to a temp directory
2. Parses the upstream TypeScript registry and help objects to extract every command and its flags
3. Parses this repo's Go command registry and implementations to extract the same
4. Diffs the two and generates a structured markdown report
5. Optionally writes the report into the `## Command Parity` section of `TODO.md`

## How to run

```bash
# Clone upstream to temp (skips LFS)
UPSTREAM=$(mktemp -d)/just-bash
GIT_LFS_SKIP_SMUDGE=1 git clone --depth 1 https://github.com/vercel-labs/just-bash.git "$UPSTREAM"

# Run the diff (prints to stdout)
python3 .claude/skills/upstream-diff/scripts/diff_commands.py "$UPSTREAM" .

# Or update TODO.md directly
python3 .claude/skills/upstream-diff/scripts/diff_commands.py "$UPSTREAM" . --update-todo

# Or get raw JSON for programmatic use
python3 .claude/skills/upstream-diff/scripts/diff_commands.py "$UPSTREAM" . --json
```

If cloning fails (e.g., LFS issues), check `/tmp` for existing checkouts — any
directory containing `src/commands/registry.ts` works as the upstream path.

## Output format

The script produces a `## Command Parity` section with two sub-sections:

- **Missing Commands** — commands in upstream that don't exist in this repo, with
  a sample of upstream flags shown as a hint for implementation scope
- **Missing Flags** — commands that exist in both repos, but where upstream supports
  flags this repo doesn't yet implement

Each item is a markdown checkbox so progress can be tracked directly in TODO.md.

When `--update-todo` is used, the script replaces the existing `## Command Parity`
section (or inserts one before `## Intentional Non-Goals`) so it's safe to re-run
repeatedly without duplication.

## How the diff script works

**Upstream extraction** parses:
- `src/commands/registry.ts` for the full command name list
- `*Help` objects in each command's `.ts` file for flag definitions (the `options` array)

**Go extraction** parses:
- `commands/registry.go` for registered commands
- `Name()` methods (both literal returns and field-based names) for command names
- Help text constants and argument parsing code for flag definitions
- Cross-file call graphs (e.g., `head.go` → `head_tail.go`) to capture shared flag parsing

## Limitations

- Flag extraction is heuristic — it may miss flags parsed in unusual ways or report
  false positives for string literals that look like flags but aren't
- The upstream help objects are the source of truth for upstream flags; commands
  without help objects won't have flag data
- Some upstream commands (python3, js-exec, node) are runtime-specific and may be
  intentionally excluded from this Go port
