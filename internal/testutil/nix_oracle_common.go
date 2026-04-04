package testutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type nixOracleConfig struct {
	envVar          string
	displayName     string
	versionLabel    string
	versionContains string
	instructions    string
	unsetErr        error
}

func requireNixOracle(tb testing.TB, cfg *nixOracleConfig, skipIfUnset bool) string {
	tb.Helper()

	path, firstLine, err := resolveNixOracle(tb.Context(), cfg)
	if err != nil {
		if skipIfUnset && errors.Is(err, cfg.unsetErr) {
			tb.Skipf("%v\n\n%s", err, cfg.instructions)
		}
		tb.Fatalf("%v\n\n%s", err, cfg.instructions)
	}
	tb.Logf("%s oracle: %s (%s)", cfg.displayName, firstLine, path)
	return path
}

func resolveNixOracle(ctx context.Context, cfg *nixOracleConfig) (path, firstLine string, err error) {
	path = strings.TrimSpace(os.Getenv(cfg.envVar)) //nolint:forbidigo // Tests explicitly read the oracle path from the host env.
	if path == "" {
		return "", "", cfg.unsetErr
	}

	out, err := exec.CommandContext(ctx, path, "--version").Output() //nolint:forbidigo // Tests validate the configured external oracle before use.
	if err != nil {
		return "", "", fmt.Errorf("failed to get %s version from %s: %w", cfg.displayName, path, err)
	}

	firstLine, _, _ = strings.Cut(string(out), "\n")
	if !strings.Contains(firstLine, cfg.versionContains) {
		return "", "", fmt.Errorf(
			"tests require %s %s (pinned via Nix), got: %s",
			cfg.displayName,
			cfg.versionLabel,
			firstLine,
		)
	}

	return path, firstLine, nil
}
