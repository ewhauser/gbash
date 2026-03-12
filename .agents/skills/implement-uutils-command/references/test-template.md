# Test Template

Tests live in `runtime/` as integration tests (not in `commands/`). Use the helpers from
`runtime/test_helpers_test.go` and `runtime/fixture_helpers_test.go`.

## Helpers

| Helper | Signature | Purpose |
|--------|-----------|---------|
| `newSession` | `newSession(t, &Config{})` | Create a test session with default config |
| `mustExecSession` | `mustExecSession(t, session, script)` | Execute script, fail test on error |
| `writeSessionFile` | `writeSessionFile(t, session, path, data)` | Write file to session's virtual FS |
| `readSessionFile` | `readSessionFile(t, session, path)` | Read file from session |

## Test file naming

Add tests to an existing `runtime/<category>_commands_test.go` file when the command fits.
Only create a new file for a genuinely new category.

## Template

```go
package runtime

import (
    "strings"
    "testing"
)

func TestCommandBasic(t *testing.T) {
    session := newSession(t, &Config{})
    writeSessionFile(t, session, "/home/agent/input.txt", []byte("line1\nline2\nline3\n"))

    result := mustExecSession(t, session, "mycommand /home/agent/input.txt\n")
    if result.ExitCode != 0 {
        t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
    }
    if got, want := result.Stdout, "expected\n"; got != want {
        t.Fatalf("Stdout = %q, want %q", got, want)
    }
}

func TestCommandFlag(t *testing.T) {
    session := newSession(t, &Config{})
    writeSessionFile(t, session, "/tmp/in.txt", []byte("data\n"))

    result := mustExecSession(t, session, "mycommand -f value /tmp/in.txt\n")
    if result.ExitCode != 0 {
        t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
    }
    // assert expected behavior
}

func TestCommandStdinPipe(t *testing.T) {
    session := newSession(t, &Config{})

    result := mustExecSession(t, session, "echo 'hello world' | mycommand\n")
    if result.ExitCode != 0 {
        t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
    }
}

func TestCommandMissingFile(t *testing.T) {
    session := newSession(t, &Config{})

    result := mustExecSession(t, session, "mycommand /nonexistent\n")
    if result.ExitCode == 0 {
        t.Fatalf("ExitCode = 0, want nonzero for missing file")
    }
    if !strings.Contains(result.Stderr, "No such file") {
        t.Fatalf("Stderr = %q, want file-not-found message", result.Stderr)
    }
}

func TestCommandInvalidFlag(t *testing.T) {
    session := newSession(t, &Config{})

    result := mustExecSession(t, session, "mycommand --bogus\n")
    if result.ExitCode == 0 {
        t.Fatalf("ExitCode = 0, want nonzero for invalid flag")
    }
}
```

## Assertion patterns

```go
// Exact stdout match
if got, want := result.Stdout, "expected\n"; got != want {
    t.Fatalf("Stdout = %q, want %q", got, want)
}

// Partial match
if !strings.Contains(result.Stdout, "expected") {
    t.Fatalf("Stdout = %q, want to contain %q", result.Stdout, "expected")
}

// Expected failure with specific error message
if result.ExitCode == 0 {
    t.Fatalf("ExitCode = 0, want nonzero")
}
if !strings.Contains(result.Stderr, "expected error") {
    t.Fatalf("Stderr = %q, want error message", result.Stderr)
}

// Check a written file
data := readSessionFile(t, session, "/tmp/output.txt")
if string(data) != "expected content\n" {
    t.Fatalf("output = %q, want %q", data, "expected content\n")
}
```
