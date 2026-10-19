package nodejs

import (
	"strings"
	"testing"
)

func TestNodeJSFSOptionObjectsTolerateMissingKeys(t *testing.T) {
	t.Parallel()

	result := runNodeScript(t, strings.Join([]string{
		`const fs = require('fs');`,
		`fs.mkdirSync('/tmp/a/b/c', { recursive: true });`,
		`fs.mkdirSync('/tmp/a/b/c', { recursive: true });`,
		`fs.mkdirSync('/tmp/m', { mode: 0o700 });`,
		`fs.mkdirSync('/tmp/n', 0o750);`,
		`fs.mkdirSync('/tmp/e', {});`,
		`fs.writeFileSync('/tmp/a/f.txt', 'hello', {});`,
		`console.log(fs.readFileSync('/tmp/a/f.txt', {}).toString());`,
		`console.log(fs.readFileSync('/tmp/a/f.txt', { encoding: 'utf8' }));`,
		`console.log(fs.readFileSync('/tmp/a/f.txt', { flag: 'r' }).length);`,
		`console.log(fs.readdirSync('/tmp/a', {}).join(','));`,
		`fs.rmSync('/tmp/missing', { force: true });`,
		`fs.rmSync('/tmp/a/f.txt', {});`,
		`fs.rmSync('/tmp/a', { recursive: true });`,
		`fs.rmdirSync('/tmp/e', {});`,
		`console.log((fs.statSync('/tmp/m').mode & 0o777).toString(8), (fs.statSync('/tmp/n').mode & 0o777).toString(8));`,
		`console.log(fs.existsSync('/tmp/a'), fs.existsSync('/tmp/e'));`,
	}, "\n")+"\n")
	if got, want := result.Stdout, "hello\nhello\n5\nb,f.txt\n700 750\nfalse false\nafter=0\n"; got != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", got, want, result.Stderr)
	}
}

func TestNodeJSMkdirModeOnlyHonorsPermissionBits(t *testing.T) {
	t.Parallel()

	result := runNodeScript(t, strings.Join([]string{
		`const fs = require('fs');`,
		`fs.mkdirSync('/tmp/d', { mode: 134217728 + 493 });`,
		`const st = fs.lstatSync('/tmp/d');`,
		`console.log(st.isDirectory(), st.isSymbolicLink(), (st.mode & 0o777).toString(8));`,
		`fs.rmSync('/tmp/d', { recursive: true });`,
		`console.log(fs.existsSync('/tmp/d'));`,
		`for (const mode of [-1, 1.5, Infinity, '9', 'abc']) {`,
		`  try { fs.mkdirSync('/tmp/bad', { mode }); console.log('created', mode); } catch (e) { console.log('rejected', String(mode)); }`,
		`}`,
		`console.log(fs.existsSync('/tmp/bad'));`,
	}, "\n")+"\n")
	want := "true false 755\nfalse\nrejected -1\nrejected 1.5\nrejected Infinity\nrejected 9\nrejected abc\nfalse\nafter=0\n"
	if result.Stdout != want {
		t.Fatalf("Stdout = %q, want %q; stderr=%q", result.Stdout, want, result.Stderr)
	}
}
