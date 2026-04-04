package testutil

import "testing"

func TestResolveNixYQ(t *testing.T) {
	t.Parallel()

	runResolveNixOracleTests(t, &nixOracleTestConfig{
		name:          "resolveNixYQ",
		envVar:        nixYQEnv,
		unsetErr:      errNixYQUnset,
		resolve:       resolveNixYQ,
		version:       pinnedNixYQVersion,
		wrongVersion:  "yq (https://github.com/mikefarah/yq/) version v4.52.5",
		successLine:   "yq (https://github.com/mikefarah/yq/) version v4.52.4",
		binaryName:    "yq",
		requirePrefix: "tests require yq ",
	})
}
