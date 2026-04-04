package testutil

import (
	"context"
	"errors"
	"testing"
)

const (
	nixJQEnv             = "GBASH_CONFORMANCE_JQ"
	pinnedNixJQVersion   = "1.8.1"
	pinnedNixJQSubstring = "jq-" + pinnedNixJQVersion
)

var errNixJQUnset = errors.New(nixJQEnv + " is not set")

// RequireNixJQ returns the pinned jq oracle configured for the test suite,
// failing the test when it is unavailable or misconfigured.
func RequireNixJQ(tb testing.TB) string {
	return requireNixOracle(tb, nixJQConfig(), false)
}

// RequireNixJQOrSkip returns the pinned jq oracle configured for the test
// suite, skipping the test when it is unavailable. If the env var is set but
// points at the wrong jq, the test fails so misconfiguration is surfaced
// immediately.
func RequireNixJQOrSkip(tb testing.TB) string {
	return requireNixOracle(tb, nixJQConfig(), true)
}

func resolveNixJQ(ctx context.Context) (path, firstLine string, err error) {
	return resolveNixOracle(ctx, nixJQConfig())
}

func nixJQInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_JQ to the pinned Nix jq:\n" +
		"  export GBASH_CONFORMANCE_JQ=$(./scripts/ensure-jq.sh)"
}

func nixJQConfig() *nixOracleConfig {
	return &nixOracleConfig{
		envVar:          nixJQEnv,
		displayName:     "jq",
		versionLabel:    pinnedNixJQVersion,
		versionContains: pinnedNixJQSubstring,
		instructions:    nixJQInstructions(),
		unsetErr:        errNixJQUnset,
	}
}
