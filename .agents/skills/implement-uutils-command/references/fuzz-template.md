# Fuzz Test Template

Fuzz tests are mandatory for every command — the sandbox is a security boundary.

## Where to add

Add to `runtime/fuzz_command_targets_test.go`. If the command fits an existing fuzz function's
category, extend it. Otherwise create a new `Fuzz<Category>Commands` function.

Existing categories:
- `FuzzFilePathCommands` — file ops (touch, cp, mv, ln, stat, etc.)
- `FuzzDirectoryTraversalCommands` — directory tree ops (du, tree)
- `FuzzTextSearchCommands` — text processing (grep, sed, sort, uniq, cut, awk, head, tail, etc.)
- `FuzzShellProcessCommands` — shell/process helpers (tee, env, printenv, which, date, sleep)
- `FuzzNestedShellCommands` — nested execution (timeout, sh, bash, xargs)
- `FuzzDataCommands` — structured data (jq, base64, sha256sum)
- `FuzzArchiveCommands` — archive (tar, gzip, gunzip, zcat)

## Template for a new standalone fuzz target

```go
func FuzzMyCommand(f *testing.F) {
    rt := newFuzzRuntime(f)

    seeds := [][]byte{
        []byte("hello\n"),
        []byte("# title\nbody\n"),
        []byte{0x00, 0x01, 0x02, 0xff},
    }
    for _, seed := range seeds {
        f.Add(seed)
    }

    f.Fuzz(func(t *testing.T, rawData []byte) {
        session := newFuzzSession(t, rt)
        data := clampFuzzData(rawData)
        inputPath := "/tmp/input.txt"

        writeSessionFile(t, session, inputPath, data)

        script := []byte(fmt.Sprintf(
            "mycommand %s > /tmp/out.txt\n"+
                "mycommand -f value %s > /tmp/out2.txt\n",
            shellQuote(inputPath),
            shellQuote(inputPath),
        ))

        result, err := runFuzzSessionScript(t, session, script)
        assertSecureFuzzOutcome(t, script, result, err)
    })
}
```

## Template for extending an existing category

Add lines to the script in the existing fuzz function:
```go
// Inside the existing f.Fuzz callback, add to the script:
"mycommand %s > /tmp/mycommand.txt\n"+
"mycommand --flag %s > /tmp/mycommand2.txt\n"+
```

## Key helpers

| Helper | Purpose |
|--------|---------|
| `newFuzzRuntime(f)` | Runtime with tight limits (200 commands, 16KB output) |
| `newFuzzSession(t, rt)` | Session on fuzz runtime |
| `runFuzzSessionScript(t, session, script)` | Execute script with size guard and timeout |
| `clampFuzzData(data)` | Truncate to 2KB |
| `fuzzPath(name)` | Sanitize to `/tmp/<name>` |
| `shellQuote(value)` | Single-quote escape for shell |
| `normalizeFuzzText(data)` | Normalize bytes to valid UTF-8 |
| `sanitizeFuzzToken(raw)` | Clean string for use as a value |

## Oracle selection

**Use `assertSecureFuzzOutcome` for all new commands.** This is the correct default.

| Oracle | When to use |
|--------|-------------|
| `assertSecureFuzzOutcome` | **Default.** Allows non-zero exit but checks for crashes, host path leaks, sensitive disclosure, and runaway execution. |
| `assertSuccessfulFuzzExecution` | Only when the fuzz script should always exit 0. Calls `assertSecureFuzzOutcome` internally plus checks exit code. |
| `assertBaseFuzzOutcome` | Minimal. Rarely appropriate for new commands. |

The security checks verify:
1. No panics/crashes (`panic:`, `runtime error:`, `fatal error:`, `SIGSEGV`, `goroutine` dumps)
2. No host path leaks (CWD, home directory)
3. No sensitive disclosure (`$HOME`, `$USER`, `$LOGNAME`, `$SHELL`, `$TMPDIR`, hostname)
4. Execution completes within timeout

## Makefile

Add the new fuzz target to `Makefile` in the appropriate `FUZZ_FULL_SHARD_*`:
```makefile
go test ./runtime -run=^$$ -fuzz=FuzzMyCommand -fuzztime=$(FUZZTIME)
```
