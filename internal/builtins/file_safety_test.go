package builtins_test

import (
	"archive/tar"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash/policy"
)

func TestCPAndInstallCapCopiesAtMaxFileBytes(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)

	result := execWithTimeout(t, session, ""+
		"cp /dev/zero /tmp/big; echo rc=$?\n"+
		"install /dev/zero /tmp/big2; echo rc=$?\n"+
		"test -e /tmp/big2 || echo no-install-partial\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nrc=1\nno-install-partial\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "File too large") {
		t.Fatalf("Stderr = %q, want File too large", result.Stderr)
	}
}

func TestDdSkipOnVirtualDeviceDoesNotRead(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	start := time.Now()
	result := execWithTimeout(t, session, "dd if=/dev/zero skip=9223372036854775807 bs=1 count=1 status=none | wc -c\n", 500*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("dd ran for %v after a 500ms timeout", elapsed)
	}
	if got, want := strings.TrimSpace(result.Stdout), "1"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func TestBackupRefusesToDestroySource(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := execWithTimeout(t, session, ""+
		"cd /tmp\n"+
		"echo A > y~; echo B > y\n"+
		"mv -b y~ y; echo mv=$?; cat y~ y\n"+
		"install -b y~ y; echo install=$?; cat y~ y\n"+
		"ln -fb y~ y; echo ln=$?; cat y~ y\n"+
		"ln -sfb y~ y; echo lns=$?; cat y~ y\n", 10*time.Second)
	if got, want := result.Stdout, "mv=1\nA\nB\ninstall=1\nA\nB\nln=1\nA\nB\nlns=1\nA\nB\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	for _, want := range []string{
		"mv: backing up 'y' might destroy source;  'y~' not moved",
		"install: backing up '/tmp/y' might destroy source;  'y~' not copied",
		"ln: backing up 'y' might destroy source;  'y' not created",
	} {
		if !strings.Contains(result.Stderr, want) {
			t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
		}
	}
}

func TestTarCreateSkipsArchiveReachedThroughHardLink(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := execWithTimeout(t, session, ""+
		"cd /tmp && mkdir d && echo hi > d/f && : > out.tar && ln out.tar d/h\n"+
		"tar -cf out.tar d; echo rc=$?\n"+
		"tar -tf out.tar | sort\n", 10*time.Second)
	if got, want := result.Stdout, "rc=0\nd\nd/f\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "tar: d/h: file is the archive; not dumped") {
		t.Fatalf("Stderr = %q, want archive skip notice", result.Stderr)
	}
}

func TestStdoutDecompressionIsCapped(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)
	writeSessionFile(t, session, "/tmp/bomb.gz", gzipBytes(t, make([]byte, 4*fileLimitTestBytes)))
	writeSessionFile(t, session, "/tmp/bomb.tar", buildTarFixture(t, tarFixtureEntry{
		Name: "zeros.bin",
		Body: make([]byte, 4*fileLimitTestBytes),
	}))

	result := execWithTimeout(t, session, ""+
		"gzip -dc /tmp/bomb.gz | wc -c; echo rc=${PIPESTATUS[0]}\n"+
		"zcat /tmp/bomb.gz > /dev/null; echo rc=$?\n"+
		"gzip -t /tmp/bomb.gz; echo rc=$?\n"+
		"tar -xOf /tmp/bomb.tar > /dev/null; echo rc=$?\n", 10*time.Second)
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) != 5 || lines[1] != "rc=1" || lines[2] != "rc=1" || lines[3] != "rc=1" || lines[4] != "rc=1" {
		t.Fatalf("Stdout = %q, want every decompression to fail; stderr=%q", result.Stdout, result.Stderr)
	}
}

func TestTarExtractEnforcesCumulativeBudget(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)
	entries := make([]tarFixtureEntry, 20)
	for i := range entries {
		entries[i] = tarFixtureEntry{Name: "f" + strings.Repeat("x", i), Body: make([]byte, fileLimitTestBytes)}
	}
	writeSessionFile(t, session, "/tmp/many.tar", buildTarFixture(t, entries...))

	result := execWithTimeout(t, session, ""+
		"mkdir /tmp/out && tar -xf /tmp/many.tar -C /tmp/out; echo rc=$?\n"+
		"ls /tmp/out | wc -l\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\n16\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "archive exceeds extraction limit") {
		t.Fatalf("Stderr = %q, want extraction limit diagnostic", result.Stderr)
	}
}

func TestTarKeepsBackslashesInMemberNames(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/bs.tar", buildTarFixture(t,
		tarFixtureEntry{Name: "d/f", Body: []byte("real\n")},
		tarFixtureEntry{Name: `d\f`, Body: []byte("other\n")},
	))

	result := execWithTimeout(t, session, ""+
		"mkdir /tmp/out && tar -xf /tmp/bs.tar -C /tmp/out; echo rc=$?\n"+
		"cat /tmp/out/d/f; cat '/tmp/out/d\\f'\n", 10*time.Second)
	if got, want := result.Stdout, "rc=0\nreal\nother\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func TestTarRejectsSymlinkEscapingThroughSymlinkedParent(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/"},
			WriteRoots:  []string{"/"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})
	writeSessionFile(t, session, "/tmp/esc.tar", buildTarFixture(t,
		tarFixtureEntry{Name: "a/b/", Typeflag: tar.TypeDir},
		tarFixtureEntry{Name: "a/b/p", Typeflag: tar.TypeSymlink, Linkname: "../.."},
		tarFixtureEntry{Name: "a/b/p/q", Typeflag: tar.TypeSymlink, Linkname: "../secret"},
	))

	result := execWithTimeout(t, session, ""+
		"mkdir /tmp/out && tar -xf /tmp/esc.tar -C /tmp/out; echo rc=$?\n"+
		"test -L /tmp/out/q || echo no-escape-link\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nno-escape-link\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, `unsafe symlink target "../secret"`) {
		t.Fatalf("Stderr = %q, want unsafe symlink diagnostic", result.Stderr)
	}
}
