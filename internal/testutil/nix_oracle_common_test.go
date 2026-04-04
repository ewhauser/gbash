package testutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nixOracleTestConfig struct {
	name          string
	envVar        string
	unsetErr      error
	resolve       func(context.Context) (string, string, error)
	version       string
	wrongVersion  string
	successLine   string
	binaryName    string
	requirePrefix string
}

func runResolveNixOracleTests(t *testing.T, cfg *nixOracleTestConfig) {
	t.Helper()

	t.Run("missing env", func(t *testing.T) {
		t.Setenv(cfg.envVar, "")

		_, _, err := cfg.resolve(context.Background())
		if !errors.Is(err, cfg.unsetErr) {
			t.Fatalf("%s() error = %v, want %v", cfg.name, err, cfg.unsetErr)
		}
	})

	t.Run("wrong version", func(t *testing.T) {
		path := writeFakeVersionedBinary(t, cfg.binaryName, cfg.wrongVersion)
		t.Setenv(cfg.envVar, path)

		_, _, err := cfg.resolve(context.Background())
		if err == nil {
			t.Fatalf("%s() error = nil, want version error", cfg.name)
		}
		if !strings.Contains(err.Error(), cfg.requirePrefix+cfg.version) {
			t.Fatalf("%s() error = %v, want pinned version diagnostic", cfg.name, err)
		}
	})

	t.Run("success", func(t *testing.T) {
		path := writeFakeVersionedBinary(t, cfg.binaryName, cfg.successLine)
		t.Setenv(cfg.envVar, path)

		gotPath, gotFirstLine, err := cfg.resolve(context.Background())
		if err != nil {
			t.Fatalf("%s() error = %v", cfg.name, err)
		}
		if gotPath != path {
			t.Fatalf("%s() path = %q, want %q", cfg.name, gotPath, path)
		}
		if gotFirstLine != cfg.successLine {
			t.Fatalf("%s() firstLine = %q, want %q", cfg.name, gotFirstLine, cfg.successLine)
		}
	})
}

func writeFakeVersionedBinary(t *testing.T, binaryName, firstLine string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), binaryName)
	script := "#!/bin/sh\n" +
		"printf '%s\\n' " + shellQuote(firstLine) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
