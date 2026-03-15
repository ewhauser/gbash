# Third-Party Source

`third_party/` holds mirrored or vendored source that `gbash` depends on
directly.

Today this subtree contains:

- `third_party/mvdan-sh`, the tracked local fork of `mvdan/sh` used for shell
  parsing, expansion, and interpreter integration

Changes under `third_party/mvdan-sh/` must follow the patch-stack workflow in
`third_party/mvdan-sh/AGENTS.md`. Do not edit mirrored upstream files directly.
