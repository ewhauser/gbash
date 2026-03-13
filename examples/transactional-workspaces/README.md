# Transactional Workspaces

This example turns the existing `gbash` session and filesystem APIs into a narrated demo of snapshot, rollback, branching, diff, and trace-driven review.

The scenario seeds a realistic `/workspace` with raw CSVs and a buggy cleanup script, then walks through:

- creating a point-in-time snapshot
- running a destructive cleanup flow
- inspecting the resulting workspace diff and mutation journal
- restoring the original state instantly
- forking two alternative repair branches
- comparing both outcomes and promoting the winning branch

## Run

From the repository root:

```bash
go run ./examples/transactional-workspaces
```

For a shorter version of the output:

```bash
go run ./examples/transactional-workspaces --quiet
```

From the `examples/` module, you can also use:

```bash
cd examples
make run-transactional-workspaces
```

## What It Demonstrates

- `gbash.Runtime.NewSession` for isolated, branchable shell workspaces
- `Session.FileSystem()` as the escape hatch for snapshotting and direct workspace seeding
- `fs.NewSnapshot(...)` plus `fs.Overlay(...)` to implement rollback and branch creation
- execution trace events to render a file-mutation journal after a shell run
- deterministic state diffs by comparing the managed workspace tree before and after a run

The example is intentionally small, but it shows the product story clearly: shell workspaces become cheap to clone, safe to mutate, and easy to review.
