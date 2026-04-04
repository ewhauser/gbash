package testutil

import (
	"context"
	"errors"
	"testing"
)

const (
	nixAWKEnv             = "GBASH_CONFORMANCE_AWK"
	pinnedNixAWKVersion   = "5.3.2"
	pinnedNixAWKSubstring = "GNU Awk " + pinnedNixAWKVersion
)

var errNixAWKUnset = errors.New(nixAWKEnv + " is not set")

// RequireNixAWK returns the pinned awk oracle configured for the test suite,
// failing the test when it is unavailable or misconfigured.
func RequireNixAWK(tb testing.TB) string {
	return requireNixOracle(tb, nixAWKConfig(), false)
}

// RequireNixAWKOrSkip returns the pinned awk oracle configured for the test
// suite, skipping the test when it is unavailable. If the env var is set but
// points at the wrong awk, the test fails so misconfiguration is surfaced
// immediately.
func RequireNixAWKOrSkip(tb testing.TB) string {
	return requireNixOracle(tb, nixAWKConfig(), true)
}

func resolveNixAWK(ctx context.Context) (path, firstLine string, err error) {
	return resolveNixOracle(ctx, nixAWKConfig())
}

func nixAWKInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_AWK to the pinned Nix awk:\n" +
		"  export GBASH_CONFORMANCE_AWK=$(./scripts/ensure-awk.sh)"
}

func nixAWKConfig() *nixOracleConfig {
	return &nixOracleConfig{
		envVar:          nixAWKEnv,
		displayName:     "awk",
		versionLabel:    pinnedNixAWKVersion,
		versionContains: pinnedNixAWKSubstring,
		instructions:    nixAWKInstructions(),
		unsetErr:        errNixAWKUnset,
	}
}
