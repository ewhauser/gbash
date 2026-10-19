package builtins_test

import (
	"strings"
	"testing"
	"time"
)

func TestCPRejectsHardlinkAndSymlinkAliasesAsSameFile(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	result := execWithTimeout(t, session, ""+
		"cd /tmp && echo important > a && ln a b\n"+
		"cp a b; echo rc=$?\n"+
		"cat a\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nimportant\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "cp: 'a' and 'b' are the same file") {
		t.Fatalf("Stderr = %q, want same-file diagnostic", result.Stderr)
	}

	follow := newFollowSession(t)
	result = execWithTimeout(t, follow, ""+
		"cd /tmp && echo important > a && ln -s a link\n"+
		"cp a link; echo rc=$?\n"+
		"cat a\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nimportant\n"; got != want {
		t.Fatalf("follow Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}

	result = execWithTimeout(t, session, ""+
		"cd /tmp && echo important > i && ln i j\n"+
		"install i j; echo rc=$?\n"+
		"cat i\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nimportant\n"; got != want {
		t.Fatalf("install Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func TestCPBackupRefusesToReplaceDirectoryBackup(t *testing.T) {
	t.Parallel()

	session := newSession(t, &Config{})
	result := execWithTimeout(t, session, ""+
		"cd /tmp && echo new > src && echo old > dst\n"+
		"mkdir dst~ && echo keep > dst~/data\n"+
		"cp -b src dst; echo rc=$?\n"+
		"cat dst~/data dst\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nkeep\nold\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "cp: cannot backup 'dst': Is a directory") {
		t.Fatalf("Stderr = %q, want backup diagnostic", result.Stderr)
	}
}

func TestCPRecursiveRejectsCopyIntoItselfThroughSymlink(t *testing.T) {
	t.Parallel()

	session := newFollowSession(t)
	start := time.Now()
	result := execWithTimeout(t, session, ""+
		"cd /tmp && mkdir -p d && echo x > d/f && ln -s d dlink\n"+
		"cp -r d dlink/sub; echo rc=$?\n"+
		"test -e d/sub || echo no-copy\n", 10*time.Second)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cp ran for %v", elapsed)
	}
	if got, want := result.Stdout, "rc=1\nno-copy\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "cp: cannot copy a directory, 'd', into itself, 'dlink/sub'") {
		t.Fatalf("Stderr = %q, want into-itself diagnostic", result.Stderr)
	}
}
