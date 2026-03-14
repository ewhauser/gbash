# internal/builtins/ Working Notes

This directory contains gbash's shipped builtin command implementations.

Before changing code here, review @commands/AGENTS.md for the stable command
authoring contract and reusable helper surface.

Rules for work in `internal/builtins/`:

- Implement shipped commands here, not in `commands/`.
- If a helper or API would be useful to external command authors, move it into
  `commands/` and document it there instead of leaving it builtin-only.
- Prefer the shared `commands` authoring surface: `Invocation`, `RunCommand`,
  `ParseCommandSpec`, `ReadAll`, `ReadAllStdin`, and `inv.FS.ReadFile`.
- Keep builtin behavior aligned with sandbox policy, tracing, and size limits.
- Treat `commands/AGENTS.md` as the source of truth for the public authoring
  boundary; this file only describes how `internal/builtins/` relates to it.
