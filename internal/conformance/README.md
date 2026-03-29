This package contains the `gbash` conformance harness for vendored OILS shell coverage plus local curl parity specs.

- Shared selected bash corpus: `oils/`
- Local curl corpus: `curl/`
- Shared helper commands: `bin/`
- Shared fixtures: `fixtures/`
- Suite manifest with `skip` and `xfail` entries: `manifest.json`
- Vendored upstream source: `upstream/oils/spec/`

The helper scripts in `bin/` are shell-script replacements for the upstream Python helpers from OILS, and the vendored fixtures under `fixtures/spec/testdata` are patched to call the local `.sh` names so they still run without Python in the sandbox.
The harness honors upstream `## compare_shells:` metadata when loading OILS files, and `make conformance-test` runs the supported shell suites against pinned `bash`, `dash`, `mksh`, and `zsh` oracles. The `dash` oracle is used for `gbash`'s supported `sh` variant.
The same directory also contains small local compatibility helpers, such as `tac`, for cases where the oracle host shell may not provide a command that `gbash` already implements.
The full vendored conformance corpus is gated behind `GBASH_RUN_CONFORMANCE=1` so the default `make test` path can keep using `-race`. Known skips and expected mismatches are tracked in `manifest.json`.
Run the default pinned shell/curl suites locally with `make conformance-test`.
