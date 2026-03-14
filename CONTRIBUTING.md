# Contributing to gbash

## Repository Structure

The repo is a committed Go workspace plus a pnpm workspace. The root module has the public `gbash` package, CLI, internal runtime implementation, and core commands. [`contrib/`](./contrib/) and [`examples/`](./examples/) remain separate Go modules to keep optional dependencies out of the core import graph, while [`packages/`](./packages/) contains publishable JavaScript packages such as [`@ewhauser/gbash-wasm`](./packages/gbash-wasm/).

The repo uses both [`go.work`](./go.work) and committed child-module `replace` directives. `go.work` keeps the workspace coherent at the repo root, and the child-module replaces make each nested module buildable on its own while still declaring real tagged dependencies for published consumption.

## Building and Testing

`make build`, `make test`, and `make lint` cover the Go modules. See the [`Makefile`](./Makefile) for additional targets.

Use `npm exec --yes pnpm@10.10.0 -- install --frozen-lockfile` at the repo root when you need the JavaScript workspace dependencies, or `pnpm install` if you already manage pnpm locally.

## Module Versioning

Published versions are coordinated with the root module release line. The root module uses plain tags like `v0.0.7`; contrib modules use nested-module tags like `contrib/jq/v0.0.7` and `contrib/sqlite3/v0.0.7`. The child modules keep real version requirements in `go.mod`, plus committed local `replace` directives so the repo still builds against the local checkout during development. The shipped `gbash-extras` binary lives under the `contrib/extras` module and follows that coordinated version line.

Use `make fix-modules MODULE_VERSION=vX.Y.Z` when preparing the next coordinated root, contrib, and `@ewhauser/gbash-wasm` release line. That updates the nested module requirements, refreshes the local replaces, updates the npm package version, and runs `go mod tidy` in each child Go module.

## Release Process

The supported release path is GitHub Actions driven:

1. Run `make release` or dispatch the `Prepare Release` workflow manually.
2. Review and merge the generated `release/vX.Y.Z` PR into `main`.
3. Let the `Publish Release` workflow create the root plus contrib tags and publish the root GitHub release automatically, including both `gbash` and `gbash-extras` archives plus a shared checksum file.

`Prepare Release` derives the next release line by taking the latest root `v*` tag and incrementing the patch number.

`make tag-release RELEASE_VERSION=vX.Y.Z` remains available as a local fallback for debugging or manual recovery, but it is no longer the primary release path.

## Benchmarks

Run the local comparison benchmark from the repo root:

```bash
make bench-compare
```

Write the same report to JSON with:

```bash
make bench-compare JSON_OUT=bench-compare.json
```

The comparison report includes five runtimes:

- `gbash`: the native Go helper process
- `GNU bash`: the host `bash` interpreter launched with profiles disabled
- `gbash-extras`: the shipped extras CLI with `awk`, `jq`, `sqlite3`, and `yq` pre-registered
- `gbash-node-wasm`: the `packages/gbash-wasm/wasm` artifact booted inside Node.js
- `just-bash`: the published npm package invoked through `npx`

Before timed trials begin, the harness runs one untimed primer launch for each
runtime and scenario. That keeps the reported numbers from being dominated by the
fresh-binary first-exec penalty on macOS while still measuring a fresh process for
every timed trial.

`workspace_inventory` still uses the same generated fixture for every runtime. The
native helpers mount that fixture directly, while `gbash-node-wasm` preloads the
fixture into the in-memory `js/wasm` filesystem because host-backed filesystems are
not available there. The shared command is intentionally pipe-free so it also runs
on the current `js/wasm` target.

These numbers are a local reference point, not a portability guarantee. Startup
comparisons are still not fully apples to apples, because `just-bash` currently
embeds tools like Python in its base container and `gbash` does not.

When JSON output is enabled, each runtime result includes `artifact_size_bytes`.
For the native runtimes this is the built executable size, for `GNU bash` it is the
host `bash` executable size, for Node/WASM it is the `gbash.wasm` size, and for
`just-bash` it is the installed `node_modules` closure size measured from a
temporary `npm install` plus the host `node` executable size.

## GNU Coreutils Compatibility Testing

You can evaluate the skew between implemented commands and [coreutils](https://www.gnu.org/software/coreutils/).

Prepare the pinned GNU source tree:

```bash
make gnu-test-setup
```

Fetch the prepared GNU build cache for your platform:

```bash
make gnu-build-cache-fetch
```

Run the full configured harness or limit it to selected utilities. `make gnu-test` prefers a prepared GNU build archive from the local cache, then the dedicated GitHub Release cache, and only falls back to a local `configure && make` when no prepared archive is available:

```bash
make gnu-test
make gnu-test GNU_UTILS="printf pwd"
```

Useful overrides:

- `GNU_UTILS` limits the utility list.
- `GNU_TESTS` runs exact GNU test files instead of the manifest-selected utility suites.
- `GNU_KEEP_WORKDIR=1` preserves the temporary patched/build workdir.
- `GNU_FORCE_REBUILD=1` bypasses any prepared archive and forces a fresh local GNU build.

Maintainers can refresh the published prepared build archive set with:

```bash
make gnu-build-cache-publish
```

The scheduled GitHub compatibility workflow runs inside the published compat image from GitHub Container Registry. Local `make compat-docker-build` and `make compat-docker-run` now pull that published image by default, tag it locally as `gbash-compat-local`, and only fall back to a full local rebuild if the published image is unavailable.

```bash
make compat-docker-run
```

Force a full local rebuild when you need to bypass the published image:

```bash
COMPAT_DOCKER_BASE_IMAGE= COMPAT_DOCKER_PULL=0 make compat-docker-build
```
