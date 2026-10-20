package shell

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gbfs "github.com/ewhauser/gbash/fs"
	"github.com/ewhauser/gbash/internal/builtins"
	"github.com/ewhauser/gbash/internal/shell/interp"
	"github.com/ewhauser/gbash/policy"
)

func runBudgetScript(t *testing.T, fsys gbfs.FileSystem, limits *policy.Limits, script string) (stdout, stderr string, code int) {
	t.Helper()
	if fsys == nil {
		fsys = newBudgetTestFS(t)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var out, errOut strings.Builder
	_, err := Run(ctx, &Execution{
		Script:   script,
		Env:      map[string]string{"PATH": "/bin"},
		Registry: builtins.DefaultRegistry(),
		FS:       fsys,
		Policy: policy.NewStatic(&policy.Config{
			ReadRoots:  []string{"/"},
			WriteRoots: []string{"/"},
			Limits:     *limits,
		}),
		Stdout: &out,
		Stderr: &errOut,
	})
	var status interp.ExitStatus
	switch {
	case err == nil:
	case errors.As(err, &status):
		code = int(status)
	default:
		t.Fatalf("Run() error = %v stderr=%q", err, errOut.String())
	}
	if ctx.Err() != nil {
		t.Fatalf("Run() did not finish before the deadline; stderr=%q", errOut.String())
	}
	return out.String(), errOut.String(), code
}

func newBudgetTestFS(t *testing.T) gbfs.FileSystem {
	t.Helper()
	return newShellTestFS(t, "echo", "cat", "kill", "seq", "sleep")
}

func TestLoopGuardCannotBeSpoofedByUserCode(t *testing.T) {
	t.Parallel()
	limits := &policy.Limits{MaxLoopIterations: 5, MaxCommandCount: 1000}

	_, stderr, code := runBudgetScript(t, nil, limits, "i=0; while (( 1 )); do __jb_loop_iter for x$((i++)); done\n")
	if code != 2 || !strings.Contains(stderr, "__jb_loop_iter: reserved command name") {
		t.Fatalf("literal helper: code=%d stderr=%q, want reserved-name rejection", code, stderr)
	}

	stdout, stderr, code := runBudgetScript(t, nil, limits, "c=__jb_loop_iter; while (( 1 )); do $c for x; done\necho unreachable\n")
	if code != 126 || !strings.Contains(stderr, "too many iterations") || stdout != "" {
		t.Fatalf("dynamic helper: code=%d stdout=%q stderr=%q, want loop limit", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "__jb_loop_iter: command not found") {
		t.Fatalf("dynamic helper stderr = %q, want command not found", stderr)
	}
}

func TestEvalSourceAndTrapBodiesAreCompiled(t *testing.T) {
	t.Parallel()
	limits := &policy.Limits{MaxLoopIterations: 5, MaxCommandCount: 1000}

	for name, script := range map[string]string{
		"eval":   "eval 'i=0; while (( 1 )); do (( i++ )); done'\necho unreachable\n",
		"source": "echo 'while :; do :; done' > /loop.sh\nsource /loop.sh\necho unreachable\n",
		"trap":   "trap 'while :; do :; done' EXIT\n:\n",
	} {
		stdout, stderr, code := runBudgetScript(t, nil, limits, script)
		if code != 126 || !strings.Contains(stderr, "too many iterations") || stdout != "" {
			t.Errorf("%s: code=%d stdout=%q stderr=%q, want loop limit", name, code, stdout, stderr)
		}
	}

	_, stderr, code := runBudgetScript(t, nil, limits, "eval 'coproc cat'\necho status=$?\n")
	if code != 0 || !strings.Contains(stderr, "coprocesses are not supported") {
		t.Fatalf("eval validation: code=%d stderr=%q, want coproc rejection", code, stderr)
	}
}

func TestExecOutputProcSubstDoesNotHang(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := runBudgetScript(t, nil, &policy.Limits{}, "exec > >(cat); echo hello\n")
	if code != 0 || stdout != "hello\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want hello", code, stdout, stderr)
	}
	stdout, _, _ = runBudgetScript(t, nil, &policy.Limits{}, "exec 3< <(echo in); read -u 3 x; echo \"[$x]\"\n")
	if stdout != "[in]\n" {
		t.Fatalf("input procsubst stdout = %q, want [in]", stdout)
	}
}

func TestBackgroundJobsDoNotOutliveRun(t *testing.T) {
	t.Parallel()
	fsys := newBudgetTestFS(t)
	_, stderr, code := runBudgetScript(t, fsys, &policy.Limits{MaxLoopIterations: 1 << 30, MaxCommandCount: 1 << 30},
		"{ i=0; while (( i < 1000000 )); do echo x >> /bgout; (( i++ )); done; } &\necho started\n")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	info, err := fsys.Stat(context.Background(), "/bgout")
	if err != nil {
		return // canceled before the first write
	}
	size := info.Size()
	time.Sleep(50 * time.Millisecond)
	info, err = fsys.Stat(context.Background(), "/bgout")
	if err != nil || info.Size() != size {
		t.Fatalf("background job kept writing after Run returned: size %d -> %v (err %v)", size, info, err)
	}
}

func TestBackgroundJobsAreCapped(t *testing.T) {
	t.Parallel()
	_, stderr, _ := runBudgetScript(t, nil, &policy.Limits{MaxCommandCount: 100000},
		"f() { sleep 1; }\nfor i in $(seq 300); do f & done\n")
	if !strings.Contains(stderr, "background job limit reached") {
		t.Fatalf("stderr = %q, want background job limit", stderr)
	}
}

func TestGlobBudgetChargedAtRuntime(t *testing.T) {
	t.Parallel()
	fsys := newBudgetTestFS(t)
	if err := fsys.MkdirAll(context.Background(), "/d", 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runBudgetScript(t, fsys, &policy.Limits{MaxGlobOperations: 5, MaxLoopIterations: 100, MaxCommandCount: 1000},
		"for i in 1 2 3 4 5; do echo /d/*; done\necho unreachable\n")
	if code != 126 || !strings.Contains(stderr, "Glob operation limit exceeded") || strings.Contains(stdout, "unreachable") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want glob limit", code, stdout, stderr)
	}

	_, stderr, code = runBudgetScript(t, fsys, &policy.Limits{MaxGlobOperations: 5, MaxCommandCount: 1000},
		"p='/d/*'; for i in 1 2 3 4 5; do echo $p; done\n")
	if code != 126 || !strings.Contains(stderr, "Glob operation limit exceeded") {
		t.Fatalf("dynamic pattern: code=%d stderr=%q, want glob limit", code, stderr)
	}
}

func TestEvalAndSourceCountTowardFunctionDepth(t *testing.T) {
	t.Parallel()
	limits := &policy.Limits{MaxFunctionDepth: 20, MaxCommandCount: 100000}
	for name, script := range map[string]string{
		"eval":   "f() { eval f; }\nf\necho unreachable\n",
		"source": "echo 'source /s' > /s\nsource /s\necho unreachable\n",
	} {
		stdout, stderr, code := runBudgetScript(t, nil, limits, script)
		if code != 126 || !strings.Contains(stderr, "maximum function nesting level exceeded (20)") || stdout != "" {
			t.Errorf("%s: code=%d stdout=%q stderr=%q, want function depth limit", name, code, stdout, stderr)
		}
	}
}

func TestKillNonCatchableSignalReportsError(t *testing.T) {
	t.Parallel()
	stdout, stderr, _ := runBudgetScript(t, nil, &policy.Limits{}, "kill -9 $$\necho rc=$?\n")
	if stdout != "rc=1\n" || !strings.Contains(stderr, "cannot send SIGKILL") {
		t.Fatalf("stdout=%q stderr=%q, want kill failure", stdout, stderr)
	}
}

func TestCoprocRejectedBeforeRunning(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := runBudgetScript(t, nil, &policy.Limits{}, "echo first\ncoproc cat\n")
	if code != 2 || !strings.Contains(stderr, "coproc: coprocesses are not supported") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want coproc rejection", code, stdout, stderr)
	}
}
