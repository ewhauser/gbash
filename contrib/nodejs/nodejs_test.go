package nodejs

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	gbruntime "github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/policy"
	"github.com/ewhauser/gbash/trace"
)

func TestRegisterAddsNodeJSCommand(t *testing.T) {
	t.Parallel()

	registry := newNodeRegistry(t)
	if !slices.Contains(registry.Names(), "nodejs") {
		t.Fatalf("Names() missing %q: %v", "nodejs", registry.Names())
	}
	if slices.Contains(commands.DefaultRegistry().Names(), "nodejs") {
		t.Fatalf("DefaultRegistry() should not include nodejs")
	}
}

func TestNodeJSEvalSupportsGlobalsAndArgs(t *testing.T) {
	t.Parallel()

	result, err := newNodeGBRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script: `nodejs -e "console.log(Buffer.from('hi').toString()); console.log(process.argv.join(','))" -- one two` + "\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "hi\nnodejs,one,two\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestNodeJSFileExecutionSupportsCommonJSGlobals(t *testing.T) {
	t.Parallel()

	session := newNodeSession(t)
	writeSessionFile(t, session, "/home/agent/lib/dep.js", []byte("exports.value = 'ok';\n"))
	writeSessionFile(t, session, "/home/agent/main.js", []byte(""+
		"const dep = require('./lib/dep');\n"+
		"console.log(__filename);\n"+
		"console.log(__dirname);\n"+
		"console.log(dep.value);\n"))

	result := mustExecNodeSession(t, session, "nodejs ./main.js\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "/home/agent/main.js\n/home/agent\nok\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestNodeJSReadsFromStdin(t *testing.T) {
	t.Parallel()

	result, err := newNodeGBRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script: "printf \"console.log('stdin')\\n\" | nodejs\n",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "stdin\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestNodeJSOverridesSandboxSensitiveBuiltins(t *testing.T) {
	t.Parallel()

	script := "" +
		`nodejs -e "console.log(require('process') === require('node:process'));` +
		`console.log(require('console') === require('node:console'));` +
		`console.log(require('fs') === require('node:fs'));` +
		`console.log(require('path') === require('node:path'))"` + "\n"

	result, err := newNodeGBRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{Script: script})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "true\ntrue\ntrue\ntrue\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func TestNodeJSRuntimeIsolationAcrossExecs(t *testing.T) {
	t.Parallel()

	session := newNodeSession(t)

	first := mustExecNodeSession(t, session, "nodejs -e \"globalThis.marker = 1; process.env.EXTRA = 'x'; console.log(globalThis.marker); console.log(process.env.EXTRA)\"\n")
	if first.ExitCode != 0 {
		t.Fatalf("first ExitCode = %d, want 0; stderr=%q", first.ExitCode, first.Stderr)
	}
	if got, want := first.Stdout, "1\nx\n"; got != want {
		t.Fatalf("first Stdout = %q, want %q", got, want)
	}

	second := mustExecNodeSession(t, session, "nodejs -e \"console.log(globalThis.marker === undefined); console.log(process.env.EXTRA === undefined)\"\n")
	if second.ExitCode != 0 {
		t.Fatalf("second ExitCode = %d, want 0; stderr=%q", second.ExitCode, second.Stderr)
	}
	if got, want := second.Stdout, "true\ntrue\n"; got != want {
		t.Fatalf("second Stdout = %q, want %q", got, want)
	}
}

func TestNodeJSRejectsUnsupportedModulesAndNodeModulesLookup(t *testing.T) {
	t.Parallel()

	session := newNodeSession(t)
	writeSessionFile(t, session, "/home/agent/node_modules/pkg/index.js", []byte("exports.value = 'bad';\n"))

	unsupported := mustExecNodeSession(t, session, "nodejs -e \"require('http')\"\n")
	if unsupported.ExitCode == 0 {
		t.Fatalf("unsupported ExitCode = %d, want non-zero", unsupported.ExitCode)
	}
	if !strings.Contains(unsupported.Stderr, `unsupported module "http"`) {
		t.Fatalf("unsupported stderr = %q, want unsupported module message", unsupported.Stderr)
	}

	nodeModules := mustExecNodeSession(t, session, "nodejs -e \"require('pkg')\"\n")
	if nodeModules.ExitCode == 0 {
		t.Fatalf("node_modules ExitCode = %d, want non-zero", nodeModules.ExitCode)
	}
	if !strings.Contains(nodeModules.Stderr, `unsupported module "pkg"`) {
		t.Fatalf("node_modules stderr = %q, want unsupported module message", nodeModules.Stderr)
	}
}

func TestNodeJSRejectsDirectoryModules(t *testing.T) {
	t.Parallel()

	session := newNodeSession(t)
	writeSessionFile(t, session, "/home/agent/lib/index.js", []byte("exports.value = 'bad';\n"))
	writeSessionFile(t, session, "/home/agent/main.js", []byte("require('./lib');\n"))

	result := mustExecNodeSession(t, session, "nodejs ./main.js\n")
	if result.ExitCode == 0 {
		t.Fatalf("ExitCode = %d, want non-zero", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "directory modules are not supported") {
		t.Fatalf("Stderr = %q, want directory-module rejection", result.Stderr)
	}
}

func TestNodeJSFileSystemUsesSandboxPolicyAndTraces(t *testing.T) {
	t.Parallel()

	registry := newNodeRegistry(t)
	rt, err := gbruntime.New(gbruntime.WithConfig(&gbruntime.Config{
		Registry: registry,
		Policy: policy.NewStatic(&policy.Config{
			AllowedCommands: []string{"nodejs"},
			ReadRoots:       []string{"/allowed", "/tmp", "/usr/bin", "/bin", "/home/agent"},
			WriteRoots:      []string{"/tmp", "/usr/bin", "/bin", "/home/agent"},
			Limits: policy.Limits{
				MaxStdoutBytes: 1 << 20,
				MaxStderrBytes: 1 << 20,
				MaxFileBytes:   8 << 20,
			},
			NetworkMode: policy.NetworkDisabled,
			SymlinkMode: policy.SymlinkDeny,
		}),
	}))
	if err != nil {
		t.Fatalf("runtime.New() error = %v", err)
	}

	session, err := rt.NewSession(context.Background())
	if err != nil {
		t.Fatalf("Runtime.NewSession() error = %v", err)
	}

	writeSessionFile(t, session, "/allowed/input.txt", []byte("ok\n"))
	writeSessionFile(t, session, "/denied.txt", []byte("secret\n"))

	allowed := mustExecNodeSession(t, session, ""+
		"nodejs -e \"const fs = require('fs'); fs.writeFileSync('/tmp/out.txt', fs.readFileSync('/allowed/input.txt', 'utf8')); console.log(fs.readFileSync('/tmp/out.txt', 'utf8'))\"\n")
	if allowed.ExitCode != 0 {
		t.Fatalf("allowed ExitCode = %d, want 0; stderr=%q", allowed.ExitCode, allowed.Stderr)
	}
	if got, want := allowed.Stdout, "ok\n\n"; got != want {
		t.Fatalf("allowed Stdout = %q, want %q", got, want)
	}
	if !hasFileAccess(allowed.Events, "read", "/allowed/input.txt") {
		t.Fatalf("allowed events missing read access: %#v", allowed.Events)
	}
	if !hasFileAccess(allowed.Events, "write", "/tmp/out.txt") {
		t.Fatalf("allowed events missing write access: %#v", allowed.Events)
	}

	denied := mustExecNodeSession(t, session, "nodejs -e \"require('fs').readFileSync('/denied.txt', 'utf8')\"\n")
	if denied.ExitCode == 0 {
		t.Fatalf("denied ExitCode = %d, want non-zero", denied.ExitCode)
	}
	if !strings.Contains(denied.Stderr, `outside allowed roots`) {
		t.Fatalf("denied stderr = %q, want sandbox denial", denied.Stderr)
	}
	if !hasPolicyPath(denied.Events, "/denied.txt") {
		t.Fatalf("denied events missing policy path: %#v", denied.Events)
	}
}

func TestNodeJSTimesOut(t *testing.T) {
	t.Parallel()

	result, err := newNodeGBRuntime(t).Run(context.Background(), &gbruntime.ExecutionRequest{
		Script:  "nodejs -e \"while (true) {}\"\n",
		Timeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 124 {
		t.Fatalf("ExitCode = %d, want 124; stderr=%q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "execution timed out") {
		t.Fatalf("Stderr = %q, want timeout marker", result.Stderr)
	}
}

func TestNodeJSShebangViaEnvWorks(t *testing.T) {
	t.Parallel()

	session := newNodeSession(t)
	writeSessionFile(t, session, "/home/agent/tool.js", []byte(""+
		"#!/usr/bin/env nodejs\n"+
		"console.log('shebang');\n"))

	result := mustExecNodeSession(t, session, "/home/agent/tool.js\n")
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if got, want := result.Stdout, "shebang\n"; got != want {
		t.Fatalf("Stdout = %q, want %q", got, want)
	}
}

func hasFileAccess(events []trace.Event, action, name string) bool {
	for _, event := range events {
		if event.Kind != trace.EventFileAccess || event.File == nil {
			continue
		}
		if event.File.Action == action && event.File.Path == name {
			return true
		}
	}
	return false
}

func hasPolicyPath(events []trace.Event, name string) bool {
	for _, event := range events {
		if event.Kind != trace.EventPolicyDenied || event.Policy == nil {
			continue
		}
		if event.Policy.Path == name {
			return true
		}
	}
	return false
}
