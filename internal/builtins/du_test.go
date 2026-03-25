package builtins_test

import (
	"context"
	"strings"
	"testing"
)

func TestDUVisiblePathAndSymlinkModes(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	mustExecSession(t, session, "mkdir -p /tmp/dir/1/2\n")
	if err := session.FileSystem().Symlink(context.Background(), "dir", "/tmp/slink"); err != nil {
		t.Fatalf("Symlink(slink) error = %v", err)
	}

	result := mustExecSession(t, session, strings.Join([]string{
		"cd /tmp",
		"du slink | cut -f2-",
		"printf '%s\\n' ---",
		"du -D slink | cut -f2-",
		"printf '%s\\n' ---",
		"du slink/ | cut -f2-",
		"printf '%s\\n' ---",
		"du -L slink | cut -f2-",
	}, "\n"))
	if got, want := result.ExitCode, 0; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}

	const want = "slink\n---\nslink/1/2\nslink/1\nslink\n---\nslink/1/2\nslink/1\nslink/\n---\nslink/1/2\nslink/1\nslink\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestDUHardLinksDedupAndCountLinks(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/dir/f1", []byte("payload\n"))
	mustExecSession(t, session, "mkdir -p /tmp/dir/sub\n")
	if err := session.FileSystem().Link(context.Background(), "/tmp/dir/f1", "/tmp/dir/f2"); err != nil {
		t.Fatalf("Link(f2) error = %v", err)
	}
	if err := session.FileSystem().Symlink(context.Background(), "f1", "/tmp/dir/f3"); err != nil {
		t.Fatalf("Symlink(f3) error = %v", err)
	}

	result := mustExecSession(t, session, strings.Join([]string{
		"cd /tmp",
		"du -a -L dir | cut -f2- | sed 's/f[123]/f_/' | sort",
		"printf '%s\\n' ---",
		"du -a -l -L dir | cut -f2- | sort",
		"printf '%s\\n' ---",
		"du -a -L dir dir | cut -f2- | sed 's/f[123]/f_/' | sort",
	}, "\n"))
	if got, want := result.ExitCode, 0; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}

	const want = "dir\ndir/f_\n---\ndir\ndir/f1\ndir/f2\ndir/f3\n---\ndir\ndir/f_\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestDUFiles0FromDeduplicatesAndReportsZeroLengthEntries(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/a", []byte("a"))
	writeSessionFile(t, session, "/tmp/b", []byte("bb"))
	writeSessionFile(t, session, "/tmp/list0", []byte("a\x00\x00b\x00a"))

	result := mustExecSession(t, session, strings.Join([]string{
		"cd /tmp",
		"du --apparent-size --block-size=1 --files0-from=list0 > /tmp/out",
		"status=$?",
		"cut -f2- /tmp/out",
		"printf 'status=%s\\n' \"$status\"",
	}, "\n"))
	if got, want := result.ExitCode, 0; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}
	if got, want := result.Stdout, "a\nb\nstatus=1\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if got, want := result.Stderr, "du: list0:2: invalid zero-length file name\n"; got != want {
		t.Fatalf("Stderr = %q, want %q", got, want)
	}
}

func TestDUExcludeAndExcludeFrom(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	mustExecSession(t, session, "mkdir -p /tmp/a/b/c /tmp/a/x/y /tmp/a/u/v\n")
	writeSessionFile(t, session, "/tmp/excl", []byte("b\n"))

	result := mustExecSession(t, session, strings.Join([]string{
		"cd /tmp",
		"du --exclude=x a | cut -f2- | sort",
		"printf '%s\\n' ---",
		"du --exclude-from=excl a | cut -f2- | sort",
		"printf '%s\\n' ---",
		"du --exclude=a a",
		"printf '%s\\n' ---",
		"du --exclude=a/u --exclude=a/b a | cut -f2- | sort",
	}, "\n"))
	if got, want := result.ExitCode, 0; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}

	const want = "a\na/b\na/b/c\na/u\na/u/v\n---\na\na/u\na/u/v\na/x\na/x/y\n---\n---\na\na/x\na/x/y\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestDUInodesThresholdAndWarnings(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/d/f", []byte("x"))
	mustExecSession(t, session, "mkdir -p /tmp/d/sub\n")
	if err := session.FileSystem().Link(context.Background(), "/tmp/d/f", "/tmp/d/h"); err != nil {
		t.Fatalf("Link(h) error = %v", err)
	}

	result := mustExecSession(t, session, strings.Join([]string{
		"cd /tmp",
		"du --inodes d",
		"printf '%s\\n' ---",
		"du --inodes -l d",
		"printf '%s\\n' ---",
		"du --inodes --threshold=3 d",
		"printf '%s\\n' ---",
		"du --inodes --threshold=-2 d",
		"printf '%s\\n' ---",
		"du --inodes -b d",
	}, "\n"))
	if got, want := result.ExitCode, 0; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}

	const want = "1\td/sub\n3\td\n---\n1\td/sub\n4\td\n---\n3\td\n---\n1\td/sub\n---\n1\td/sub\n3\td\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
	if !strings.Contains(result.Stderr, "ineffective with --inodes") {
		t.Fatalf("Stderr = %q, want ineffective warning", result.Stderr)
	}
}

func TestDUMaxDepthAndUnreadableDirectoryContinuation(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	mustExecSession(t, session, strings.Join([]string{
		"mkdir -p /tmp/a/b/c/d",
		"mkdir -p /tmp/f/a /tmp/f/b /tmp/f/c /tmp/f/d /tmp/f/e",
		"touch /tmp/f/c/j",
		"chmod 000 /tmp/f/c",
	}, "\n"))

	maxDepth := mustExecSession(t, session, "cd /tmp\ndu -d 1 a | cut -f2-\n")
	if got, want := maxDepth.ExitCode, 0; got != want {
		t.Fatalf("maxDepth.ExitCode = %d, want %d; stderr=%q", got, want, maxDepth.Stderr)
	}
	if got, want := maxDepth.Stdout, "a/b\na\n"; got != want {
		t.Fatalf("maxDepth.Stdout = %q, want %q", got, want)
	}

	result := mustExecSession(t, session, strings.Join([]string{
		"cd /tmp/f",
		"du > /tmp/out 2> /tmp/err",
		"status=$?",
		"cut -f2- /tmp/out | sort",
		"printf '%s\\n' ---",
		"cat /tmp/err",
		"printf 'status=%s\\n' \"$status\"",
	}, "\n"))
	if got, want := result.ExitCode, 0; got != want {
		t.Fatalf("ExitCode = %d, want %d; stderr=%q", got, want, result.Stderr)
	}

	const want = ".\n./a\n./b\n./c\n./d\n./e\n---\ndu: cannot read directory './c': Permission denied\nstatus=1\n"
	if got := result.Stdout; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}
