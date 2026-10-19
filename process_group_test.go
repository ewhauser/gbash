package gbash_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/ewhauser/gbash"
	"github.com/ewhauser/gbash/commands"
	"github.com/ewhauser/gbash/host"
	"github.com/ewhauser/gbash/internal/shellstate"
)

func processGroupRuntime(t *testing.T, opts ...gbash.Option) *gbash.Runtime {
	t.Helper()
	registry := gbash.DefaultRegistry()
	if err := registry.Register(commands.DefineCommand("pgrp", func(ctx context.Context, inv *commands.Invocation) error {
		pgrp, _ := shellstate.ProcessGroupFromContext(ctx)
		_, err := inv.Stdout.Write([]byte(strconv.Itoa(pgrp)))
		return err
	})); err != nil {
		t.Fatalf("Register(pgrp) error = %v", err)
	}
	rt, err := gbash.New(append([]gbash.Option{gbash.WithRegistry(registry)}, opts...)...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return rt
}

func runProcessGroup(t *testing.T, rt *gbash.Runtime) string {
	t.Helper()
	result, err := rt.Run(context.Background(), &gbash.ExecutionRequest{Script: "pgrp\n"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, stderr=%q", result.ExitCode, result.Stderr)
	}
	return result.Stdout
}

func TestVirtualHostReportsVirtualProcessGroup(t *testing.T) {
	t.Parallel()
	if got := runProcessGroup(t, processGroupRuntime(t)); got != "1" {
		t.Fatalf("virtual host process group = %q, want %q (no host pgrp leak)", got, "1")
	}
}

func TestSystemHostReportsHostProcessGroup(t *testing.T) {
	t.Parallel()
	meta, err := host.NewSystem().ExecutionMeta(context.Background())
	if err != nil {
		t.Fatalf("ExecutionMeta() error = %v", err)
	}
	if got, want := runProcessGroup(t, processGroupRuntime(t, gbash.WithHost(host.NewSystem()))), strconv.Itoa(meta.ProcessGroup); got != want {
		t.Fatalf("system host process group = %q, want %q", got, want)
	}
}
