# JavaScript Packages

`packages/` holds publishable JavaScript and TypeScript artifacts that are
versioned alongside the Go runtime.

Today that includes:

- `packages/gbash-wasm` for the browser and Node host bindings around the
  `js/wasm` build of `gbash`
- `packages/gbash-wasm/wasm` for the Go `js/wasm` entrypoint that exposes the
  `GBashWasm` browser API

These packages are part of the repository's distribution story, not the core Go
embedding API.
