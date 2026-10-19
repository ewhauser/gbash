package builtins_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ewhauser/gbash/policy"
)

const fileLimitTestBytes = 64 << 10

func newFileLimitSession(t *testing.T) *Session {
	t.Helper()
	return newSession(t, &Config{
		LimitOverrides: policy.Limits{MaxFileBytes: fileLimitTestBytes},
	})
}

func execWithTimeout(t *testing.T, session *Session, script string, timeout time.Duration) *ExecutionResult {
	t.Helper()
	result, err := session.Exec(context.Background(), &ExecutionRequest{Script: script, Timeout: timeout})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	return result
}

func TestTruncateRejectsSizesBeyondMaxFileBytes(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)

	result := execWithTimeout(t, session, ""+
		"truncate -s 9223372036854775807 /tmp/huge; echo rc=$?\n"+
		"truncate -s 1M /tmp/big; echo rc=$?\n"+
		"wc -c < /tmp/huge; wc -c < /tmp/big\n"+
		"truncate -s 1K /tmp/ok; echo rc=$?; wc -c < /tmp/ok\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nrc=1\n0\n0\nrc=0\n1024\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "truncate: failed to truncate '/tmp/huge' at 9223372036854775807 bytes: File too large") {
		t.Fatalf("Stderr = %q, want File too large diagnostic", result.Stderr)
	}
}

func TestDdRejectsHugeBlockSizesAndSeeksWithoutAborting(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)

	result := execWithTimeout(t, session, ""+
		"dd of=/tmp/a seek=1 bs=64T count=0 status=none; echo rc=$?\n"+
		"dd if=/dev/null of=/tmp/b seek=9223372036854775807 bs=1 count=0 status=none; echo rc=$?\n"+
		"dd if=/dev/null of=/tmp/c bs=9223372036854775807 count=0 status=none; echo rc=$?\n"+
		"echo after\n", 10*time.Second)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (script must keep running); stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "rc=1\nrc=1\nrc=1\nafter\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	for _, want := range []string{
		"dd: bs=N cannot fit into memory",
		"dd: failed to truncate to 9223372036854775807 bytes in output file '/tmp/b': File too large",
	} {
		if !strings.Contains(result.Stderr, want) {
			t.Fatalf("Stderr = %q, want %q", result.Stderr, want)
		}
	}
	if strings.Contains(result.Stderr, "panic") {
		t.Fatalf("Stderr = %q, want no panic", result.Stderr)
	}
}

func TestDdStopsWritingAtMaxFileBytes(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)

	result := execWithTimeout(t, session, ""+
		"head -c 70000 /dev/zero | dd of=/tmp/out bs=4096 status=none; echo rc=$?\n"+
		"head -c 4096 /dev/zero | dd of=/tmp/small bs=1024 status=none; echo rc=$?; wc -c < /tmp/small\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nrc=0\n4096\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "dd: error writing '/tmp/out': File too large") {
		t.Fatalf("Stderr = %q, want write diagnostic", result.Stderr)
	}
}

func TestDdObservesRequestTimeout(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	start := time.Now()
	result := execWithTimeout(t, session, "dd if=/dev/zero of=/dev/null bs=1 status=none\n", 200*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("dd ran for %v after a 200ms timeout", elapsed)
	}
	if result.ExitCode == 0 {
		t.Fatalf("ExitCode = 0, want timeout failure")
	}
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	return buf.Bytes()
}

func TestGunzipCapsDecompressedOutput(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)
	writeSessionFile(t, session, "/tmp/bomb.gz", gzipBytes(t, make([]byte, 4*fileLimitTestBytes)))
	writeSessionFile(t, session, "/tmp/ok.gz", gzipBytes(t, []byte("hello\n")))

	result := execWithTimeout(t, session, ""+
		"gunzip /tmp/bomb.gz; echo rc=$?\n"+
		"test -e /tmp/bomb || echo no-partial\n"+
		"test -e /tmp/bomb.gz && echo source-kept\n"+
		"gunzip /tmp/ok.gz; echo rc=$?; cat /tmp/ok\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nno-partial\nsource-kept\nrc=0\nhello\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "File too large") {
		t.Fatalf("Stderr = %q, want File too large", result.Stderr)
	}
}

func TestTarCapsExtractedFileSize(t *testing.T) {
	t.Parallel()
	session := newFileLimitSession(t)
	writeSessionFile(t, session, "/tmp/bomb.tgz", buildTarGzipFixture(t, tarFixtureEntry{
		Name: "zeros.bin",
		Body: make([]byte, 4*fileLimitTestBytes),
	}))

	result := execWithTimeout(t, session, ""+
		"mkdir -p /tmp/out && tar -xzf /tmp/bomb.tgz -C /tmp/out; echo rc=$?\n"+
		"test -e /tmp/out/zeros.bin || echo absent\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nabsent\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "File too large") {
		t.Fatalf("Stderr = %q, want File too large", result.Stderr)
	}
}

func TestSplitRejectsHugeChunkCounts(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})

	result := execWithTimeout(t, session, ""+
		"cd /tmp && printf 'abc\\n' > in\n"+
		"split -n 1000000000 in part; echo rc=$?\n"+
		"ls | grep -c '^part' || true\n"+
		"split -e -n 1000000000 in elided; echo rc=$?\n"+
		"ls | grep -c '^elided'\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\n0\nrc=0\n4\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "too many output files") {
		t.Fatalf("Stderr = %q, want too many output files", result.Stderr)
	}
}

func TestTarSymlinkEntryDoesNotDeleteNonEmptyDirectory(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/work/src/main.go", []byte("package main\n"))
	writeSessionFile(t, session, "/tmp/link.tar", buildTarFixture(t, tarFixtureEntry{
		Name:     "src",
		Typeflag: tar.TypeSymlink,
		Linkname: "x",
	}))

	result := execWithTimeout(t, session, "tar -xf /tmp/link.tar -C /tmp/work; echo rc=$?\ncat /tmp/work/src/main.go\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\npackage main\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "Cannot create symlink: File exists") {
		t.Fatalf("Stderr = %q, want File exists", result.Stderr)
	}

	// An empty directory or a plain file is still replaced.
	result = execWithTimeout(t, session, "rm -rf /tmp/work/src && mkdir /tmp/work/src && tar -xf /tmp/link.tar -C /tmp/work; echo rc=$?\nreadlink /tmp/work/src\n", 10*time.Second)
	if got, want := result.Stdout, "rc=0\nx\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func newFollowSession(t *testing.T) *Session {
	t.Helper()
	return newSession(t, &Config{
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:   []string{"/"},
			WriteRoots:  []string{"/"},
			SymlinkMode: policy.SymlinkFollow,
		}),
	})
}

func TestTarRejectsWritesThroughArchiveSymlinkChains(t *testing.T) {
	t.Parallel()
	session := newFollowSession(t)
	writeSessionFile(t, session, "/tmp/work/secret.txt", []byte("secret\n"))
	writeSessionFile(t, session, "/tmp/chain.tar", buildTarFixture(t,
		tarFixtureEntry{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "."},
		tarFixtureEntry{Name: "a/b", Typeflag: tar.TypeSymlink, Linkname: ".."},
		tarFixtureEntry{Name: "a/b/escaped.txt", Body: []byte("owned\n")},
	))
	writeSessionFile(t, session, "/tmp/hardlink.tar", buildTarFixture(t,
		tarFixtureEntry{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "."},
		tarFixtureEntry{Name: "a/b", Typeflag: tar.TypeSymlink, Linkname: ".."},
		tarFixtureEntry{Name: "stolen", Typeflag: tar.TypeLink, Linkname: "a/b/secret.txt"},
	))

	result := execWithTimeout(t, session, ""+
		"mkdir -p /tmp/work/root1 /tmp/work/root2\n"+
		"tar -xf /tmp/chain.tar -C /tmp/work/root1; echo rc=$?\n"+
		"test -e /tmp/work/escaped.txt || echo not-escaped\n"+
		"tar -xf /tmp/hardlink.tar -C /tmp/work/root2; echo rc=$?\n"+
		"test -e /tmp/work/root2/stolen || echo not-linked\n", 10*time.Second)
	if got, want := result.Stdout, "rc=1\nnot-escaped\nrc=1\nnot-linked\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "escapes extraction directory") {
		t.Fatalf("Stderr = %q, want escape diagnostic", result.Stderr)
	}

	// Symlinks that stay inside the extraction root keep working.
	writeSessionFile(t, session, "/tmp/inside.tar", buildTarFixture(t,
		tarFixtureEntry{Name: "real/", Typeflag: tar.TypeDir},
		tarFixtureEntry{Name: "alias", Typeflag: tar.TypeSymlink, Linkname: "real"},
		tarFixtureEntry{Name: "alias/file.txt", Body: []byte("ok\n")},
	))
	result = execWithTimeout(t, session, "mkdir -p /tmp/work/root3 && tar -xf /tmp/inside.tar -C /tmp/work/root3; echo rc=$?\ncat /tmp/work/root3/real/file.txt\n", 10*time.Second)
	if got, want := result.Stdout, "rc=0\nok\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func TestTarCreateDotNamesMembersAndSkipsOwnArchive(t *testing.T) {
	t.Parallel()
	session := newSession(t, &Config{})
	writeSessionFile(t, session, "/tmp/proj/a.txt", []byte("a\n"))
	writeSessionFile(t, session, "/tmp/proj/sub/b.txt", []byte("b\n"))

	result := execWithTimeout(t, session, "cd /tmp/proj && tar -cf out.tar . ; echo rc=$?\n", 10*time.Second)
	if got, want := result.Stdout, "rc=0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
	tr := tar.NewReader(bytes.NewReader(readSessionFile(t, session, "/tmp/proj/out.tar")))
	var names []string
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, header.Name)
	}
	if got, want := strings.Join(names, ","), "./,./a.txt,./sub/,./sub/b.txt"; got != want {
		t.Fatalf("member names = %q, want %q", got, want)
	}
	if !strings.Contains(result.Stderr, "tar: ./out.tar: file is the archive; not dumped") {
		t.Fatalf("Stderr = %q, want archive self-skip warning", result.Stderr)
	}
}
