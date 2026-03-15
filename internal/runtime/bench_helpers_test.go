package runtime

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
)

type benchmarkFSBackend struct {
	name  string
	new   func() gbfs.FileSystem
	clone func(gbfs.FileSystem) (gbfs.FileSystem, error)
}

type benchmarkPreparedSessionFactory struct {
	base  gbfs.FileSystem
	clone func(gbfs.FileSystem) (gbfs.FileSystem, error)
}

func (f benchmarkPreparedSessionFactory) New(context.Context) (gbfs.FileSystem, error) {
	return f.clone(f.base)
}

func (benchmarkPreparedSessionFactory) layoutReady() bool {
	return true
}

func benchmarkFSBackends() []benchmarkFSBackend {
	return []benchmarkFSBackend{
		{
			name: "memory",
			new: func() gbfs.FileSystem {
				return gbfs.NewMemory()
			},
			clone: func(fsys gbfs.FileSystem) (gbfs.FileSystem, error) {
				base, ok := fsys.(*gbfs.MemoryFS)
				if !ok {
					return nil, fmt.Errorf("clone backend %T, want *fs.MemoryFS", fsys)
				}
				return base.Clone(), nil
			},
		},
		{
			name: "trie",
			new: func() gbfs.FileSystem {
				return gbfs.NewTrie()
			},
			clone: func(fsys gbfs.FileSystem) (gbfs.FileSystem, error) {
				base, ok := fsys.(*gbfs.TrieFS)
				if !ok {
					return nil, fmt.Errorf("clone backend %T, want *fs.TrieFS", fsys)
				}
				return base.Clone(), nil
			},
		},
	}
}

func newPreparedRuntime(tb testing.TB, backend benchmarkFSBackend, files map[string]string) *Runtime {
	tb.Helper()

	rt := newRuntime(tb, nil)
	base := backend.new()
	if err := initializeSandboxLayout(context.Background(), base, rt.cfg.BaseEnv, rt.cfg.FileSystem.WorkingDir, rt.cfg.Registry.Names()); err != nil {
		tb.Fatalf("initializeSandboxLayout() error = %v", err)
	}
	seedBenchmarkFiles(tb, base, files)

	rt.sessionFactory = benchmarkPreparedSessionFactory{
		base:  base,
		clone: backend.clone,
	}
	return rt
}

func seedBenchmarkFiles(tb testing.TB, fsys gbfs.FileSystem, files map[string]string) {
	tb.Helper()

	for name, contents := range files {
		if err := fsys.MkdirAll(context.Background(), path.Dir(name), 0o755); err != nil {
			tb.Fatalf("MkdirAll(%q) error = %v", path.Dir(name), err)
		}
		file, err := fsys.OpenFile(context.Background(), name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			tb.Fatalf("OpenFile(%q) error = %v", name, err)
		}
		if _, err := file.Write([]byte(contents)); err != nil {
			_ = file.Close()
			tb.Fatalf("Write(%q) error = %v", name, err)
		}
		if err := file.Close(); err != nil {
			tb.Fatalf("Close(%q) error = %v", name, err)
		}
	}
}

func mustNewBenchmarkSession(tb testing.TB, rt *Runtime) *Session {
	tb.Helper()

	session, err := rt.NewSession(context.Background())
	if err != nil {
		tb.Fatalf("NewSession() error = %v", err)
	}
	return session
}

func mustExecRequest(tb testing.TB, session *Session, req *ExecutionRequest) *ExecutionResult {
	tb.Helper()

	result, err := session.Exec(context.Background(), req)
	if err != nil {
		tb.Fatalf("Exec() error = %v", err)
	}
	return result
}

func mustRunRequest(tb testing.TB, rt *Runtime, req *ExecutionRequest) *ExecutionResult {
	tb.Helper()

	result, err := rt.Run(context.Background(), req)
	if err != nil {
		tb.Fatalf("Run() error = %v", err)
	}
	return result
}

func requireExecutionOutcome(tb testing.TB, result *ExecutionResult, wantStdout string) {
	tb.Helper()

	if result == nil {
		tb.Fatalf("result is nil")
		return
	}
	if result.ExitCode != 0 {
		tb.Fatalf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Stderr != "" {
		tb.Fatalf("Stderr = %q, want empty stderr", result.Stderr)
	}
	if result.Stdout != wantStdout {
		tb.Fatalf("Stdout = %q, want %q", result.Stdout, wantStdout)
	}
}

func benchmarkRuntimeRun(b *testing.B, rt *Runtime, req *ExecutionRequest, wantStdout string) {
	b.Helper()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requireExecutionOutcome(b, mustRunRequest(b, rt, req), wantStdout)
	}
}

func benchmarkSessionExec(b *testing.B, session *Session, req *ExecutionRequest, wantStdout string) {
	b.Helper()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requireExecutionOutcome(b, mustExecRequest(b, session, req), wantStdout)
	}
}

func benchmarkFreshSessionExec(b *testing.B, rt *Runtime, req *ExecutionRequest, wantStdout string) {
	b.Helper()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session := mustNewBenchmarkSession(b, rt)
		requireExecutionOutcome(b, mustExecRequest(b, session, req), wantStdout)
	}
}

type benchmarkWorkflowStep struct {
	req        ExecutionRequest
	wantStdout string
}

func benchmarkWorkflow(b *testing.B, rt *Runtime, steps ...benchmarkWorkflowStep) {
	b.Helper()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session := mustNewBenchmarkSession(b, rt)
		for j := range steps {
			requireExecutionOutcome(b, mustExecRequest(b, session, &steps[j].req), steps[j].wantStdout)
		}
	}
}
