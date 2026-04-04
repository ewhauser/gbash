package testutil

import (
	"context"
	"errors"
	"testing"
)

const (
	nixBashEnv             = "GBASH_CONFORMANCE_BASH"
	pinnedNixBashVersion   = "5.3.9"
	pinnedNixBashSubstring = "version " + pinnedNixBashVersion
)

var errNixBashUnset = errors.New(nixBashEnv + " is not set")

// RequireNixBash returns the pinned bash oracle configured for the test suite,
// failing the test when it is unavailable or misconfigured.
func RequireNixBash(tb testing.TB) string {
	return requireNixOracle(tb, nixBashConfig(), false)
}

// RequireNixBashOrSkip returns the pinned bash oracle configured for
// the test suite, skipping the test when it is unavailable. If the env var is
// set but points at the wrong bash, the test fails so misconfiguration is
// surfaced immediately.
func RequireNixBashOrSkip(tb testing.TB) string {
	return requireNixOracle(tb, nixBashConfig(), true)
}

func resolveNixBash(ctx context.Context) (path, firstLine string, err error) {
	return resolveNixOracle(ctx, nixBashConfig())
}

func nixBashInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_BASH to the pinned Nix bash:\n" +
		"  export GBASH_CONFORMANCE_BASH=$(./scripts/ensure-bash.sh)"
}

func nixBashConfig() *nixOracleConfig {
	return &nixOracleConfig{
		envVar:          nixBashEnv,
		displayName:     "bash",
		versionLabel:    pinnedNixBashVersion,
		versionContains: pinnedNixBashSubstring,
		instructions:    nixBashInstructions(),
		unsetErr:        errNixBashUnset,
	}
}
