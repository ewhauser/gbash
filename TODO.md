# Legacy Option Parser Migration

This file tracks registered commands that still manually parse `inv.Args` and need to migrate to `Spec() CommandSpec` plus `RunParsed(...)`.

Audit scope:
- Include registered commands that still parse flags, help, version, or option-like operands by hand.
- Exclude helper files, tests, not-implemented stubs, and commands already using the new parser methods.
- Current remaining scope: `59` command files / `63` command entrypoints.

Already migrated and intentionally excluded from this TODO:
- `commands/checksum_sums.go`: `b2sum`, `md5sum`, `sha1sum`, `sha224sum`, `sha256sum`, `sha384sum`, `sha512sum`
- `commands/dircolors.go`: `dircolors`
- `commands/seq.go`: `seq`
- `commands/tee.go`: `tee`

## Shared or Multi-Command Migrations

- [ ] `commands/base32.go`, `commands/base64.go`: `base32`, `base64`
- [ ] `commands/bash.go`: `bash`, `sh`
- [ ] `commands/env.go`: `env`, `printenv`
- [ ] `commands/gzip.go`: `gzip`, `gunzip`, `zcat`
- [ ] `commands/head.go`, `commands/tail.go`, shared `commands/head_tail.go`: `head`, `tail`
- [ ] `commands/ls.go`, `commands/dir.go`: `ls`, `dir`

## Standalone Migrations

Unless otherwise noted above, each command name maps directly to `commands/<name>.go`.

- [ ] `basename`, `cat`, `chmod`, `chown`, `column`, `comm`
- [ ] `cp`, `curl`, `cut`, `date`, `diff`, `dirname`
- [ ] `du`, `echo`, `file`, `find`, `grep`, `id`
- [ ] `join`, `link`, `ln`, `mkdir`, `mv`, `nl`
- [ ] `od`, `paste`, `pwd`, `readlink`, `rev`, `rg`
- [ ] `rm`, `rmdir`, `sed`, `sleep`, `sort`, `split`
- [ ] `stat`, `tac`, `tar`, `timeout`, `touch`, `tr`
- [ ] `tree`, `uniq`, `uptime`, `wc`, `which`, `whoami`, `xargs`, `yes`

## Verification Checklist

- [ ] Cross-check this list against `commands/registry.go` so every registered command with manual flag/help parsing appears exactly once.
- [ ] Confirm no helper or test files are represented here, and no command is duplicated under both a shared-owner item and a standalone item.
- [ ] Confirm no command from `commands/checksum_sums.go`, `commands/dircolors.go`, `commands/seq.go`, or `commands/tee.go` appears in the migration list.

## Notes

- Commands with no real option parser today are intentionally out of scope, even if they read positional args directly.
- `find` stays in scope, but its dedicated expression parser helpers can remain; only the top-level CLI parsing needs to move onto the new spec layer unless a later pass chooses to replace the expression parser too.
- Not-implemented registry entries such as `vdir` are intentionally excluded from this file.
