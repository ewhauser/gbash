package builtins_test

import (
	"strings"
	"testing"
	"time"
)

func TestFindDeleteDoesNotFollowSymlinkedDirectories(t *testing.T) {
	t.Parallel()

	session := newFollowSession(t)
	result := execWithTimeout(t, session, ""+
		"mkdir -p /tmp/outside /tmp/tree/sub && echo keep > /tmp/outside/precious\n"+
		"echo x > /tmp/tree/sub/f && ln -s /tmp/outside /tmp/tree/link\n"+
		"find /tmp/tree -delete; echo rc=$?\n"+
		"cat /tmp/outside/precious\n"+
		"test -e /tmp/tree || echo tree-gone\n", 10*time.Second)
	if got, want := result.Stdout, "rc=0\nkeep\ntree-gone\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func TestFindFollowReportsSymlinkLoops(t *testing.T) {
	t.Parallel()

	session := newFollowSession(t)
	result := execWithTimeout(t, session, ""+
		"mkdir -p /tmp/loop/a && ln -s .. /tmp/loop/a/up\n"+
		"find -L /tmp/loop | sort; echo rc=${PIPESTATUS[0]}\n"+
		"find /tmp/loop -type l\n", 10*time.Second)
	if got, want := result.Stdout, "/tmp/loop\n/tmp/loop/a\n/tmp/loop/a/up\nrc=1\n/tmp/loop/a/up\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "find: File system loop detected; '/tmp/loop/a/up' is part of the same file system loop as '/tmp/loop'.") {
		t.Fatalf("Stderr = %q, want loop diagnostic", result.Stderr)
	}
}
