package shell

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRunHashBuiltinPrintsEmptyTable(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script:   "hash\n",
		Env:      map[string]string{"PATH": "/bin"},
		Registry: newShellTestRegistry(t),
		FS:       newShellTestFS(t),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "hash: hash table empty\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunHashBuiltinSeedsEntriesAndTracksHits(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
hash whoami _nonexistent_
echo status=$?
whoami >/dev/null
whoami >/dev/null
hash
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: newShellTestRegistry(t),
		FS:       newShellTestFS(t, "whoami", "echo"),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "status=1\nhits\tcommand\n   2\t/bin/whoami\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "hash: _nonexistent_: not found\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunHashBuiltinIgnoresExplicitPaths(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
hash /bin/whoami
echo status=$?
hash
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: newShellTestRegistry(t),
		FS:       newShellTestFS(t, "whoami", "echo"),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "status=0\nhash: hash table empty\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunHashBuiltinDashRClearsAndRehashesNames(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
hash whoami
whoami >/dev/null
hash -r whoami
echo status=$?
hash
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: newShellTestRegistry(t),
		FS:       newShellTestFS(t, "whoami", "echo"),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "status=0\nhits\tcommand\n   0\t/bin/whoami\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunSubshellHashStateIsCopied(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
hash whoami
(whoami >/dev/null; hash -r)
hash
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: newShellTestRegistry(t),
		FS:       newShellTestFS(t, "whoami"),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "hits\tcommand\n   0\t/bin/whoami\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunCachesPathLookupsUntilHashReset(t *testing.T) {
	t.Parallel()

	registry := newShellTestRegistry(t)
	fsys := newShellTestFS(t, "mkdir", "chmod", "echo")
	makeShellTmpDir(t, fsys)

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
cd /tmp
PATH="one:two:$PATH"
mkdir -p one two
echo 'echo two' > two/mycmd
chmod +x two/mycmd
mycmd
echo 'echo one' > one/mycmd
chmod +x one/mycmd
mycmd
hash -r
mycmd
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: registry,
		FS:       fsys,
		Exec:     newCoreTestExec(registry, fsys),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr=%q", err, stderr.String())
	}
	if got, want := stdout.String(), "two\ntwo\none\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunUsesStalePathCacheUntilHashReset(t *testing.T) {
	t.Parallel()

	registry := newShellTestRegistry(t)
	fsys := newShellTestFS(t, "mkdir", "chmod", "rm", "echo")
	makeShellTmpDir(t, fsys)

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
cd /tmp
PATH="one:two:$PATH"
mkdir -p one two
echo 'echo two' > two/mycmd
chmod +x two/mycmd
mycmd
echo status=$?
echo 'echo one' > one/mycmd
chmod +x one/mycmd
rm two/mycmd
mycmd
echo status=$?
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: registry,
		FS:       fsys,
		Exec:     newCoreTestExec(registry, fsys),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "two\nstatus=0\nstatus=127\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "two/mycmd: No such file or directory\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunHashBuiltinRehashesStaleEntries(t *testing.T) {
	t.Parallel()

	registry := newShellTestRegistry(t)
	fsys := newShellTestFS(t, "mkdir", "chmod", "rm", "echo")
	makeShellTmpDir(t, fsys)

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
cd /tmp
PATH="one:two:$PATH"
/bin/mkdir -p one two
echo 'echo two' > two/mycmd
/bin/chmod +x two/mycmd
hash mycmd
/bin/rm two/mycmd
echo 'echo one' > one/mycmd
/bin/chmod +x one/mycmd
hash mycmd
echo status=$?
hash
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: registry,
		FS:       fsys,
		Exec:     newCoreTestExec(registry, fsys),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "status=0\nhits\tcommand\n   0\tone/mycmd\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunPathAssignmentInvalidatesCommandHash(t *testing.T) {
	t.Parallel()

	registry := newShellTestRegistry(t)
	fsys := newShellTestFS(t, "mkdir", "chmod", "echo")
	makeShellTmpDir(t, fsys)

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
mkdir -p /tmp/bin /tmp/bin2
echo 'echo hi' > /tmp/bin/hello
echo 'echo hey' > /tmp/bin2/hello
chmod +x /tmp/bin/hello /tmp/bin2/hello
PATH="/tmp/bin:$PATH"
hello
PATH="/tmp/bin2:$PATH"
hello
PATH="/tmp/bin:$PATH" hello
PATH="/tmp/bin2:$PATH" hello
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: registry,
		FS:       fsys,
		Exec:     newCoreTestExec(registry, fsys),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v, stderr=%q", err, stderr.String())
	}
	if got, want := stdout.String(), "hi\nhey\nhi\nhey\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunCommandPBypassesCommandHash(t *testing.T) {
	t.Parallel()

	registry := newShellTestRegistry(t)
	fsys := newShellTestFS(t, "mkdir", "chmod", "echo")
	makeShellTmpDir(t, fsys)

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
/bin/mkdir -p /tmp/custom
echo 'echo custom' > /tmp/custom/foo
/bin/chmod +x /tmp/custom/foo
PATH="/tmp/custom:$PATH"
hash foo
command -p foo >/dev/null 2>/dev/null
echo status=$?
hash
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: registry,
		FS:       fsys,
		Exec:     newCoreTestExec(registry, fsys),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "status=127\nhits\tcommand\n   0\t/tmp/custom/foo\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunCommandPRefreshesCommandHash(t *testing.T) {
	t.Parallel()

	registry := newShellTestRegistry(t)
	fsys := newShellTestFS(t, "mkdir", "chmod", "rm")
	makeShellTmpDir(t, fsys)

	var stdout strings.Builder
	var stderr strings.Builder

	_, err := Run(context.Background(), &Execution{
		Script: `
/bin/mkdir -p /tmp/custom
printf '%s\n' placeholder > /tmp/custom/mkdir
/bin/chmod +x /tmp/custom/mkdir
PATH="/tmp/custom:$PATH"
hash mkdir
/bin/rm /tmp/custom/mkdir
command -p mkdir -p /tmp/from-default >/dev/null
hash
mkdir -p /tmp/plain
`,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: registry,
		FS:       fsys,
		Exec:     newCoreTestExec(registry, fsys),
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "hits\tcommand\n   1\t/bin/mkdir\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	if _, err := fsys.Stat(context.Background(), "/tmp/plain"); err != nil {
		t.Fatalf("Stat(/tmp/plain) error = %v", err)
	}
}

func makeShellTmpDir(t testing.TB, fsys interface {
	MkdirAll(context.Context, string, os.FileMode) error
}) {
	t.Helper()
	if err := fsys.MkdirAll(context.Background(), "/tmp", 0o755); err != nil {
		t.Fatalf("MkdirAll(/tmp) error = %v", err)
	}
}
