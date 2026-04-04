package testutil

import (
	"context"
	"errors"
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
	return requireNixOracle(tb, nixYQConfig(), false)
}

// RequireNixYQOrSkip returns the pinned yq oracle configured for the test
// suite, skipping the test when it is unavailable. If the env var is set but
// points at the wrong yq, the test fails so misconfiguration is surfaced
// immediately.
func RequireNixYQOrSkip(tb testing.TB) string {
	return requireNixOracle(tb, nixYQConfig(), true)
}

func resolveNixYQ(ctx context.Context) (path, firstLine string, err error) {
	return resolveNixOracle(ctx, nixYQConfig())
}

func nixYQInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_YQ to the pinned Nix yq:\n" +
		"  export GBASH_CONFORMANCE_YQ=$(./scripts/ensure-yq.sh)"
}

func nixYQConfig() *nixOracleConfig {
	return &nixOracleConfig{
		envVar:          nixYQEnv,
		displayName:     "yq",
		versionLabel:    pinnedNixYQVersion,
		versionContains: pinnedNixYQSubstring,
		instructions:    nixYQInstructions(),
		unsetErr:        errNixYQUnset,
	}
}
