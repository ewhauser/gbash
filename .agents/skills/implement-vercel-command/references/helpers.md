# Available Helper Functions

These helpers are already defined in the `commands` package. Use them directly — don't reimplement them.

## Filesystem access (`command.go`)

| Function | Signature | Purpose |
|----------|-----------|---------|
| `allowPath` | `allowPath(ctx, inv, action, name) (string, error)` | Resolve path and check policy. Returns absolute path. |
| `openRead` | `openRead(ctx, inv, name) (File, string, error)` | Resolve + open for reading. Returns file, absolute path, error. |
| `readDir` | `readDir(ctx, inv, name) ([]DirEntry, string, error)` | Resolve + list directory entries. |
| `statPath` | `statPath(ctx, inv, name) (FileInfo, string, error)` | Resolve + stat (follows symlinks). |
| `lstatPath` | `lstatPath(ctx, inv, name) (FileInfo, string, error)` | Resolve + lstat (no symlink follow). |
| `statMaybe` | `statMaybe(ctx, inv, action, name) (FileInfo, string, bool, error)` | Stat that doesn't error on not-found. Returns `(info, abs, exists, error)`. |
| `lstatMaybe` | `lstatMaybe(ctx, inv, action, name) (FileInfo, string, bool, error)` | Same as statMaybe but with lstat. |

## IO helpers (`io_helpers.go`)

| Function | Signature | Purpose |
|----------|-----------|---------|
| `readAllFile` | `readAllFile(ctx, inv, name) ([]byte, string, error)` | Read entire file contents. Returns data, absolute path, error. |
| `readAllStdin` | `readAllStdin(inv) ([]byte, error)` | Read all of stdin. |

## Source helpers (`source_helpers.go`)

| Function | Signature | Purpose |
|----------|-----------|---------|
| `readNamedInputs` | `readNamedInputs(ctx, inv, names, defaultStdin) ([]namedInput, error)` | Read multiple named files (or stdin if names is empty and defaultStdin is true). Handles `-` as stdin. Returns `[]namedInput` with `.Name`, `.Abs`, `.Data`, `.FromStdin` fields. |
| `readTwoInputs` | `readTwoInputs(ctx, inv, leftName, rightName) ([]byte, []byte, error)` | Read exactly two inputs (for diff-like commands). |

The `namedInput` struct:
```go
type namedInput struct {
    Name      string // base filename or "-"
    Abs       string // absolute path or "-"
    Data      []byte // file contents
    FromStdin bool   // true if read from stdin
}
```

## Text helpers (`text_helpers.go`)

| Function | Signature | Purpose |
|----------|-----------|---------|
| `textLines` | `textLines(data []byte) []string` | Split data into lines, stripping trailing newlines from each. |
| `writeTextLines` | `writeTextLines(w io.Writer, lines []string) error` | Write lines with newline after each. |

## Line splitting (`head_tail.go`)

| Function | Signature | Purpose |
|----------|-----------|---------|
| `splitLines` | `splitLines(data []byte) [][]byte` | Split data preserving trailing newlines on each line. Empty input returns nil. |

## File helpers (`file_helpers.go`)

| Function | Signature | Purpose |
|----------|-----------|---------|
| `ensureParentDirExists` | `ensureParentDirExists(ctx, inv, targetAbs) error` | Verify parent directory exists. |
| `copyFileContents` | `copyFileContents(ctx, inv, srcAbs, dstAbs, perm) error` | Copy one file. |
| `copyTree` | `copyTree(ctx, inv, srcAbs, dstAbs) error` | Recursive copy. |
| `writeFileContents` | `writeFileContents(ctx, inv, targetAbs, data, perm) error` | Write data to a file. |
| `resolveDestination` | `resolveDestination(ctx, inv, sourceAbs, destArg, multipleSources) (string, FileInfo, bool, error)` | Resolve a copy/move destination path. |

## Error handling (`command.go`)

```go
// Write message to stderr and return ExitError
exitf(inv, code, format, args...)

// Wrap a raw error with exit code
&ExitError{Code: 1, Err: err}

// Get exit code for policy errors (returns 126 for denied, 1 otherwise)
exitCodeForError(err)
```

## Writing files

```go
abs, err := allowPath(ctx, inv, policy.FileActionWrite, name)
if err != nil {
    return err
}
file, err := inv.FS.OpenFile(ctx, abs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
if err != nil {
    return &ExitError{Code: 1, Err: err}
}
defer func() { _ = file.Close() }()
_, err = file.Write(data)
```
