package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func runInterpScriptWithRunner(t *testing.T, src string) (*Runner, string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	runner, err := NewRunner(&RunnerConfig{
		Dir:    "/tmp",
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("NewRunner error = %v", err)
	}
	err = runner.runShellReader(context.Background(), strings.NewReader(src), "nameref-conformance-test.sh", nil)
	return runner, stdout.String(), stderr.String(), err
}

func TestNamerefFlagLifecycleMatchesBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
x=foo

ref=x

echo ref=$ref

typeset -n ref
echo ref=$ref

x=bar
echo ref=$ref

typeset +n ref
echo ref=$ref
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "ref=x\nref=foo\nref=bar\nref=x\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestNamerefIndirectExpansionMatchesBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
foo=FOO

x=foo
ref=x

echo ref=$ref
echo "!ref=${!ref}"

typeset -n ref
echo ref=$ref
echo "!ref=${!ref}"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "ref=x\n!ref=foo\nref=foo\n!ref=x\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestNamerefAssignmentMatchesBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
x=XX
y=YY

ref=x
ref=y
echo 1 ref=$ref

typeset -n ref
echo 2 ref=$ref

ref=XXXX
echo 3 ref=$ref
echo 4 y=$y
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "1 ref=y\n2 ref=YY\n3 ref=XXXX\n4 y=XXXX\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestNamerefInvalidTargetsMatchBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
set -- one two three

ref='#'
echo ref=$ref
typeset -n ref
echo hash=$ref

ref='1'
echo ref=$ref
typeset -n ref
echo one=$ref

ref2='$1'
echo ref2=$ref2
typeset -n ref2
echo dollar1=$ref2

bad=1
echo bad=$bad
typeset -n bad
echo bad=$bad
bad=foo
echo bad=$bad

typeset -n empty
echo empty=$empty
empty=x
echo empty=$empty
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "ref=#\nhash=#\nref=1\none=1\nref2=$1\ndollar1=$1\nbad=1\nbad=1\nbad=foo\nempty=\nempty=\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	const wantStderr = "typeset: `#': invalid variable name for name reference\ntypeset: `1': invalid variable name for name reference\ntypeset: `$1': invalid variable name for name reference\ntypeset: `1': invalid variable name for name reference\n"
	if stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", stderr, wantStderr)
	}
}

func TestNamerefRecursiveResolutionMatchesBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
typeset -n ref1=ref2
typeset -n ref2=ref1
echo defined
echo ref1=$ref1
echo ref2=$ref2

typeset -n a=b
typeset -n b=a
echo cycle=${a}

echo assign
ref1=z
echo status=$?
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "defined\nref1=\nref2=\ncycle=\nassign\nstatus=1\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	const wantStderr = "warning: ref1: circular name reference\nwarning: ref2: circular name reference\nwarning: a: circular name reference\nwarning: ref1: circular name reference\n"
	if stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", stderr, wantStderr)
	}
}

func TestNamerefArrayTargetsMatchBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
a=('A B' C)
typeset -n ref='a[@]'
printf '<%s>|<%s>\n' "$ref"
ref=(X Y Z)
echo status=$?
printf '<%s>\n' "${ref[@]}"
printf '<%s>|<%s>\n' "${a[@]}"

array=(X Y Z)
typeset -n elem='array[0]'
elem[0]=foo
echo nested=$?
printf '<%s>|<%s>|<%s>\n' "${array[@]}"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "<A B>|<C>\nstatus=1\n<>\n<A B>|<C>\nnested=1\n<X>|<Y>|<Z>\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	const wantStderr = "`a[@]': not a valid identifier\n`array[0]': not a valid identifier\n"
	if stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", stderr, wantStderr)
	}
}

func TestDeclarePrintersQuoteNamerefsLikeBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
test_var1=111
readonly test_var2=222
export test_var3=333
declare -n test_var4=test_var1
{
  echo '[declare]'
  declare -p test_var1 test_var2 test_var3 test_var4
  echo '[declare -pn]'
  declare -pn
}
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "[declare]\ndeclare -- test_var1=\"111\"\ndeclare -r test_var2=\"222\"\ndeclare -x test_var3=\"333\"\ndeclare -n test_var4=\"test_var1\"\n[declare -pn]\ndeclare -n test_var4=\"test_var1\"\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestReadonlyArrayIsNotModifiedThroughNamerefAppend(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
a=(1 2 3)
readonly -a a
eval 'declare -n r=a; r+=(4)'
printf '<%s>|<%s>|<%s>\n' "${a[@]}"
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "<1>|<2>|<3>\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	if stderr != "r: readonly variable\n" {
		t.Fatalf("stderr = %q, want %q", stderr, "r: readonly variable\n")
	}
}

func TestNamerefDynamicScopeMatchesBash(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runInterpScript(t, `
f3() {
  local -n ref=$1
  ref=x
}

f2() {
  f3 "$@"
}

f1() {
  local F1=F1
  echo F1=$F1
  f2 F1
  echo F1=$F1
}
f1
`)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	const wantStdout = "F1=F1\nF1=x\n"
	if stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", stdout, wantStdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestNamerefAssignmentsPreserveExplicitAttrUpdates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		check  func(*testing.T, expandVarSnapshot)
	}{
		{
			name: "export",
			script: `
x=0
declare -n r=x
export r=1
`,
			check: func(t *testing.T, snapshot expandVarSnapshot) {
				t.Helper()
				if snapshot.Str != "1" || !snapshot.Exported {
					t.Fatalf("x = %+v, want exported string value 1", snapshot)
				}
			},
		},
		{
			name: "readonly",
			script: `
x=0
declare -n r=x
readonly r=1
`,
			check: func(t *testing.T, snapshot expandVarSnapshot) {
				t.Helper()
				if snapshot.Str != "1" || !snapshot.ReadOnly {
					t.Fatalf("x = %+v, want readonly string value 1", snapshot)
				}
			},
		},
		{
			name: "integer",
			script: `
x=0
declare -n r=x
declare -i r=2+3
`,
			check: func(t *testing.T, snapshot expandVarSnapshot) {
				t.Helper()
				if snapshot.Str != "5" || !snapshot.Integer {
					t.Fatalf("x = %+v, want integer string value 5", snapshot)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner, stdout, stderr, err := runInterpScriptWithRunner(t, tc.script)
			if err != nil {
				t.Fatalf("Run error = %v", err)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}

			vr := runner.lookupVar("x")
			tc.check(t, expandVarSnapshot{
				Str:      vr.Str,
				Exported: vr.Exported,
				ReadOnly: vr.ReadOnly,
				Integer:  vr.Integer,
			})
		})
	}
}

type expandVarSnapshot struct {
	Str      string
	Exported bool
	ReadOnly bool
	Integer  bool
}
