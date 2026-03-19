package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/internal/shell/syntax"
)

func runInterpScript(t *testing.T, src string) (string, string, error) {
	t.Helper()

	file, err := syntax.NewParser().Parse(strings.NewReader(src), "varref-test.sh")
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	runner, err := NewRunner(&RunnerConfig{
		Dir:    "/tmp",
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("NewRunner error = %v", err)
	}

	err = runner.Run(context.Background(), file)
	return stdout.String(), stderr.String(), err
}

func TestPrintfVarRef(t *testing.T) {
	t.Parallel()

	stdout, _, err := runInterpScript(t, `
var=foo
printf -v $var %s 'hello there'
a=(a b c)
printf -v 'a[1]' %s 'foo'
printf '%s\n' "$foo"
printf '%s\n' "${a[@]}"
printf -v 'a[' %s 'foo'
printf 'status=%d\n' "$?"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const want = "hello there\na\nfoo\nc\nstatus=2\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestVarRefNamerefAndTests(t *testing.T) {
	t.Parallel()

	stdout, _, err := runInterpScript(t, `
typeset -A assoc=([k]=v)
key=k
test -v 'assoc[$key]'
printf 'test=%d\n' "$?"
[[ -v assoc[$key] ]]
printf 'dbracket=%d\n' "$?"
[[ -v assoc[k]z ]]
printf 'junk=%d\n' "$?"
declare -n ref='assoc[$key]'
test -R ref
printf 'refvar=%d\n' "$?"
ref=x
printf '%s\n' "${assoc[k]}"
array=(X Y Z)
typeset -n whole=array
whole[0]=xx
printf '%s\n' "${array[*]}"
typeset -n elem='array[0]'
elem[0]=foo
printf 'nested=%d\n' "$?"
printf '%s\n' "${array[*]}"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const want = "test=0\ndbracket=0\njunk=1\nrefvar=0\nx\nxx Y Z\nnested=1\nxx Y Z\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}
