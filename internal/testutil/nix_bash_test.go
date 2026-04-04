package testutil

import "testing"

func TestResolveNixBash(t *testing.T) {
	t.Parallel()

	runResolveNixOracleTests(t, &nixOracleTestConfig{
		name:          "resolveNixBash",
		envVar:        nixBashEnv,
		unsetErr:      errNixBashUnset,
		resolve:       resolveNixBash,
		version:       pinnedNixBashVersion,
		wrongVersion:  "GNU bash, version 5.2.37(1)-release",
		successLine:   "GNU bash, version 5.3.9(1)-release",
		binaryName:    "bash",
		requirePrefix: "tests require bash ",
	})
}
