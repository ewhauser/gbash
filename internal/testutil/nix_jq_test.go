package testutil

import "testing"

func TestResolveNixJQ(t *testing.T) {
	t.Parallel()

	runResolveNixOracleTests(t, &nixOracleTestConfig{
		name:          "resolveNixJQ",
		envVar:        nixJQEnv,
		unsetErr:      errNixJQUnset,
		resolve:       resolveNixJQ,
		version:       pinnedNixJQVersion,
		wrongVersion:  "jq-1.7.1",
		successLine:   "jq-1.8.1",
		binaryName:    "jq",
		requirePrefix: "tests require jq ",
	})
}
