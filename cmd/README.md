# Command Entrypoints

`cmd/` contains the Go command entrypoints that ship from the root `gbash`
module.

- `cmd/gbash` is the main sandbox CLI for local execution and embedding demos.
- `cmd/gbash-gnu` is the compatibility harness used to summarize GNU
  coreutils test results against `gbash`.

Install the main CLI with:

```bash
go install github.com/ewhauser/gbash/cmd/gbash@latest
```
