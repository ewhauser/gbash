package testutil

import (
	"context"
	"errors"
	"testing"
)

const (
	nixDiffEnv             = "GBASH_CONFORMANCE_DIFF"
	pinnedNixDiffVersion   = "3.12"
	pinnedNixDiffSubstring = "GNU diffutils) " + pinnedNixDiffVersion
)

var errNixDiffUnset = errors.New(nixDiffEnv + " is not set")

// RequireNixDiff returns the pinned GNU diff oracle configured for the test
// suite, failing the test when it is unavailable or misconfigured.
func RequireNixDiff(tb testing.TB) string {
	return requireNixOracle(tb, nixDiffConfig(), false)
}

func resolveNixDiff(ctx context.Context) (path, firstLine string, err error) {
	return resolveNixOracle(ctx, nixDiffConfig())
}

func nixDiffInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_DIFF to the pinned Nix diff:\n" +
		"  export GBASH_CONFORMANCE_DIFF=$(./scripts/ensure-diffutils.sh)"
}

func nixDiffConfig() *nixOracleConfig {
	return &nixOracleConfig{
		envVar:          nixDiffEnv,
		displayName:     "diffutils",
		versionLabel:    pinnedNixDiffVersion,
		versionContains: pinnedNixDiffSubstring,
		instructions:    nixDiffInstructions(),
		unsetErr:        errNixDiffUnset,
	}
}
