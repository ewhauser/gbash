package interp

import (
	"runtime"
	"strings"
	"testing"

	"github.com/ewhauser/gbash/shell/expand"
)

// countingEnviron counts lookups that reach the bottom of an environ chain.
type countingEnviron struct {
	gets int
}

func (e *countingEnviron) Get(string) expand.Variable { e.gets++; return expand.Variable{} }

func (e *countingEnviron) Each() expand.VarSeq {
	return func(func(string, expand.Variable) bool) {}
}

func TestOverlaySetFromDeepFunctionScopeLooksUpOnce(t *testing.T) {
	t.Parallel()

	base := &countingEnviron{}
	global := newOverlayEnviron(base, false, false)
	var env expand.WriteEnviron = global
	for range 2000 {
		scope := newScopedOverlayEnviron(env, false)
		scope.funcScope = true
		env = scope
	}

	base.gets = 0
	if err := env.Set("X", expand.Variable{Set: true, Kind: expand.String, Str: "v"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	// Each function scope used to look X up in its parent before passing the
	// write through, so one write from depth d walked the chain d times.
	if base.gets > 1 {
		t.Fatalf("Set() reached the base environ %d times, want at most 1", base.gets)
	}
	if got := global.Get("X").Str; got != "v" {
		t.Fatalf("global X = %q, want %q", got, "v")
	}
}

func TestSubshellForkCollapsesDeepEnvironChain(t *testing.T) {
	t.Parallel()

	r := newTestSubshellCOWRunner(t)
	r.setVarString("GLOBAL", "g")
	for i := range 500 {
		scope := newScopedOverlayEnviron(r.writeEnv, false)
		scope.funcScope = true
		r.writeEnv = scope
		r.setVar("LEVEL", expand.Variable{Set: true, Kind: expand.String, Str: strings.Repeat("x", i+1), Local: true})
		r = r.subshell(false)
		if environChainExceeds(r.writeEnv, maxForkChainLayers+2) {
			t.Fatalf("depth %d: forked environ chain was not collapsed", i)
		}
	}
	if got := r.lookupVar("GLOBAL").String(); got != "g" {
		t.Fatalf("GLOBAL = %q, want %q", got, "g")
	}
	if got := len(r.lookupVar("LEVEL").String()); got != 500 {
		t.Fatalf("len(LEVEL) = %d, want 500 (innermost local)", got)
	}
}

func TestDeepSubshellsKeepFunctionScopes(t *testing.T) {
	t.Parallel()

	// 40 levels of function call plus subshell stack 80 environ layers, which
	// forces the forks to collapse the chain. Locals, declare -g and dynamic
	// scope must behave exactly as with the full chain (checked against bash).
	stdout, stderr, err := runInterpScript(t, `
x=global
f() {
  local x="local$1"
  if [ "$1" -lt 40 ]; then ( f $(( $1 + 1 )) ); return; fi
  ( local y=inner; declare -g x=fromg; echo "$x $y"; g() { x=g; z=z; }; g; echo "$x $z" )
}
f 0
echo "$x"
`)
	if err != nil {
		t.Fatalf("run error = %v; stderr=%q", err, stderr)
	}
	if want := "local40 inner\ng z\nglobal\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestLongPipelineMemoryIsLinear(t *testing.T) { //nolint:paralleltest // measures process-wide allocations
	script := strings.Repeat("true | ", 4999) + "true\necho done\n"

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	stdout, stderr, err := runInterpScript(t, script)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("run error = %v; stderr=%q", err, stderr)
	}
	if stdout != "done\n" {
		t.Fatalf("stdout = %q, want %q", stdout, "done\n")
	}
	// Each stage used to clone the whole environ chain of the stage before it,
	// holding over 4 GB live for 5000 stages. Assembling PIPESTATUS still
	// churns about 650 MiB of short-lived slices, so the bound leaves room.
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<30 {
		t.Fatalf("5000-stage pipeline allocated %d MiB, want < 1 GiB", allocated>>20)
	}
}

func TestFunctionScopeLookupsSeeBindingChanges(t *testing.T) {
	t.Parallel()

	// Lookups and writes from function scopes jump straight to the scope that
	// binds a name; check they follow bindings that appear and disappear in
	// the scopes in between. The expected output is bash 5.3's.
	stdout, stderr, err := runInterpScript(t, `
x=global
show(){ echo "show:$x"; }
inner(){ show; unset x; show; x=inner; show; }
outer(){ local x=outer; inner; show; }
outer; show
deep(){ if [ "$1" -lt 40 ]; then deep $(($1+1)); else show; (show; x=sub; show); show; local x=deep; show; fi; }
deep 0; show
tmp(){ show; x=tmpset; show; }
x=prefix tmp; show
setg(){ declare -g y=gy; echo "y:$y"; local y=ly; echo "y:$y"; }
setg; echo "y:$y"
cnt=0; rec(){ cnt=$((cnt+1)); [ $cnt -lt 5 ] && rec; local z=$cnt; echo "z:$z"; }; rec
echo "cnt:$cnt z:${z-unset}"
`)
	if err != nil {
		t.Fatalf("runInterpScript() error = %v; stderr=%q", err, stderr)
	}
	want := "show:outer\nshow:global\nshow:inner\nshow:inner\nshow:inner\n" +
		"show:inner\nshow:inner\nshow:sub\nshow:inner\nshow:deep\nshow:inner\n" +
		"show:prefix\nshow:tmpset\nshow:inner\n" +
		"y:gy\ny:ly\ny:gy\n" +
		"z:5\nz:5\nz:5\nz:5\nz:5\ncnt:5 z:unset\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}
