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

const (
	nixYQEnv             = "GBASH_CONFORMANCE_YQ"
	pinnedNixYQVersion   = "4.52.4"
	pinnedNixYQSubstring = "version v" + pinnedNixYQVersion
)

var errNixYQUnset = errors.New(nixYQEnv + " is not set")

// RequireNixYQ returns the pinned yq oracle configured for the test suite,
// failing the test when it is unavailable or misconfigured.
func RequireNixYQ(tb testing.TB) string {
	tb.Helper()

	path, firstLine, err := resolveNixYQ(tb.Context())
	if err != nil {
		tb.Fatalf("%v\n\n%s", err, nixYQInstructions())
	}
	tb.Logf("yq oracle: %s (%s)", firstLine, path)
	return path
}

// RequireNixYQOrSkip returns the pinned yq oracle configured for the test
// suite, skipping the test when it is unavailable. If the env var is set but
// points at the wrong yq, the test fails so misconfiguration is surfaced
// immediately.
func RequireNixYQOrSkip(tb testing.TB) string {
	tb.Helper()

	path, firstLine, err := resolveNixYQ(tb.Context())
	if err != nil {
		if errors.Is(err, errNixYQUnset) {
			tb.Skipf("%v\n\n%s", err, nixYQInstructions())
		}
		tb.Fatalf("%v\n\n%s", err, nixYQInstructions())
	}
	tb.Logf("yq oracle: %s (%s)", firstLine, path)
	return path
}

func resolveNixYQ(ctx context.Context) (path, firstLine string, err error) {
	path = strings.TrimSpace(os.Getenv(nixYQEnv)) //nolint:forbidigo // Tests explicitly read the oracle yq path from the host env.
	if path == "" {
		return "", "", errNixYQUnset
	}

	out, err := exec.CommandContext(ctx, path, "--version").Output() //nolint:forbidigo // Tests validate the configured external yq oracle before use.
	if err != nil {
		return "", "", fmt.Errorf("failed to get yq version from %s: %w", path, err)
	}

	firstLine, _, _ = strings.Cut(string(out), "\n")
	if !strings.Contains(firstLine, pinnedNixYQSubstring) {
		return "", "", fmt.Errorf(
			"tests require yq %s (pinned via Nix), got: %s",
			pinnedNixYQVersion,
			firstLine,
		)
	}

	return path, firstLine, nil
}

func nixYQInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_YQ to the pinned Nix yq:\n" +
		"  export GBASH_CONFORMANCE_YQ=$(./scripts/ensure-yq.sh)"
}
