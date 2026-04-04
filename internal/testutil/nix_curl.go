package testutil

import (
	"context"
	"errors"
	"testing"
)

const (
	nixCurlEnv             = "GBASH_CONFORMANCE_CURL"
	pinnedNixCurlVersion   = "8.18.0"
	pinnedNixCurlSubstring = "curl " + pinnedNixCurlVersion
)

var errNixCurlUnset = errors.New(nixCurlEnv + " is not set")

// RequireNixCurl returns the pinned curl oracle configured for the test suite,
// failing the test when it is unavailable or misconfigured.
func RequireNixCurl(tb testing.TB) string {
	return requireNixOracle(tb, nixCurlConfig(), false)
}

// RequireNixCurlOrSkip returns the pinned curl oracle configured for the test
// suite, skipping the test when it is unavailable. If the env var is set but
// points at the wrong curl, the test fails so misconfiguration is surfaced
// immediately.
func RequireNixCurlOrSkip(tb testing.TB) string {
	return requireNixOracle(tb, nixCurlConfig(), true)
}

func resolveNixCurl(ctx context.Context) (path, firstLine string, err error) {
	return resolveNixOracle(ctx, nixCurlConfig())
}

func nixCurlInstructions() string {
	return "From the repo root, set GBASH_CONFORMANCE_CURL to the pinned Nix curl:\n" +
		"  export GBASH_CONFORMANCE_CURL=$(./scripts/ensure-curl.sh)"
}

func nixCurlConfig() *nixOracleConfig {
	return &nixOracleConfig{
		envVar:          nixCurlEnv,
		displayName:     "curl",
		versionLabel:    pinnedNixCurlVersion,
		versionContains: pinnedNixCurlSubstring,
		instructions:    nixCurlInstructions(),
		unsetErr:        errNixCurlUnset,
	}
}
