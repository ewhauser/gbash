# Repository Guidelines

## Project Overview
`gbash` is a Go workspace for a deterministic shell runtime. `contrib/` houses optional heavyweight modules (`sqlite3`, `jq`, `yq`, `extras`). Read `SPEC.md` before changing runtime boundaries or sandbox behavior.

## Build & Test
Use Go 1.26+. The full workspace build/test command spans multiple modules:

```sh
make build
make test
```

`make test` resolves the pinned GNU `diff` and `ripgrep` oracles through Nix via `./scripts/ensure-diffutils.sh` and `./scripts/ensure-ripgrep.sh`. Set `GBASH_CONFORMANCE_DIFF` or `GBASH_CONFORMANCE_RIPGREP` if you need to point tests at already-installed pinned binaries.

Before submitting or updating a PR, run `make lint` from the repo root and fix any reported issues.

## Conformance & Bats Tests

Conformance tests compare gbash behavior against pinned bash, dash, mksh, zsh, and curl oracles. `dash` stands in for the supported `sh` variant. Bats tests validate shell scripts in `scripts/`. The `diff` and `ripgrep` oracle tests and these suites require Nix installed to fetch pinned binaries unless you provide the corresponding override env vars.

```sh
make conformance-test   # compares gbash vs pinned bash/dash/mksh/zsh/curl oracles
make bats-test          # scripts/ test suite

# Run a single conformance test file
make conformance-test CONFORMANCE_RUN='TestConformance/bash/oils/append.test.sh'
```

## Key Project Rules
- Unknown commands must never fall through to the host OS.
- Match the registry pattern in `commands/` when adding new built-in commands.
- For runtime changes, test exit codes, stdout/stderr, and sandboxed filesystem effects.
- If a change touches shell semantics or policy, add a regression test in `runtime/` or the relevant package.

## SPEC Sync
`SPEC.md` is the product and architecture contract. Update it in the same turn when:
- adding/removing built-in commands
- changing sandbox guarantees, policy defaults, or filesystem abstractions
- changing `mvdan/sh` integration strategy
- expanding scope, roadmap, or introducing new public packages/interfaces

Read the relevant `SPEC.md` sections before editing code, and update them once the design is clear. When in doubt, prefer a small SPEC update over silent drift.

## Commits & PRs
Use [Conventional Commits](https://www.conventionalcommits.org/) with the package as the scope and a short, imperative subject scoped to one change (e.g., `fix(runtime): normalize command-not-found errors`, `feat(fs): add snapshot export`). PR titles must follow the same format: PRs are squash-merged, so the title becomes the commit on `main`, and release-please reads those commits to build the changelog and pick the next version. Use `feat` or `fix` for user-visible changes (both bump the patch version while gbash is below 1.0), `feat!`/`fix!` or a `BREAKING CHANGE:` footer for breaking changes, and `docs`, `test`, `refactor`, `ci`, `build`, or `chore` for changes that should not trigger a release. PRs should explain user-visible behavior, note any SPEC updates, include trace/CLI output when changing execution behavior, and only be submitted after a clean local `make lint`.
