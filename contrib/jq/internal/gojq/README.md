This directory contains the minimal local `gojq` fork used by `contrib/jq`.

- Upstream: `github.com/itchyny/gojq`
- Forked from: `v0.12.18` (library sources, `builtin.jq` and root package tests; `builtin.go` is generated upstream from `builtin.jq`)
- Local change: recursive helpers that walk values (comparison, encoding, `contains`, `flatten`, object `*` merging, and `setpath`/`delpaths` updates) stop at `MaxDepth` and report a `DepthLimitError` instead of overflowing the goroutine stack, which would crash the whole host process. Helpers without an error return panic with the error and `env.Next` turns the panic into the iterator's final value. See `depth.go`.
- Local change: strings built by repetition, concatenation, joining or encoding stay within `MaxStringBytes`, and arrays built by collection, concatenation, flattening, `split` or `explode` stay within `MaxArrayLength`; exceeding either raises an uncatchable `SizeLimitError`. See `size.go`.

`LICENSE.upstream.txt` preserves the upstream license text.
