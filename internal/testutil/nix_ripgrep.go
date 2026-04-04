package testutil

import (
	"context"
	"errors"
	"testing"
)

const (
	nixRipgrepEnv             = "GBASH_CONFORMANCE_RIPGREP"
	pinnedNixRipgrepVersion   = "15.1.0"
	pinnedNixRipgrepSubstring = "ripgrep " + pinnedNixRipgrepVersion
)

var errNixRipgrepUnset = errors.New(nixRipgrepEnv + " is not set")

// RequireNixRipgrep returns the pinned ripgrep oracle configured for the test
// suite, failing the test when it is unavailable or misconfigured.
func RequireNixRipgrep(tb testing.TB) string {
	return requireNixOracle(tb, nixRipgrepConfig(), false)
}

func resolveNixRipgrep(ctx context.Context) (path, firstLine string, err error) {
	return resolveNixOracle(ctx, nixRipgrepConfig())
}

func nixRipgrepInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_RIPGREP to the pinned Nix ripgrep:\n" +
		"  export GBASH_CONFORMANCE_RIPGREP=$(./scripts/ensure-ripgrep.sh)"
}

func nixRipgrepConfig() *nixOracleConfig {
	return &nixOracleConfig{
		envVar:          nixRipgrepEnv,
		displayName:     "ripgrep",
		versionLabel:    pinnedNixRipgrepVersion,
		versionContains: pinnedNixRipgrepSubstring,
		instructions:    nixRipgrepInstructions(),
		unsetErr:        errNixRipgrepUnset,
	}
}
