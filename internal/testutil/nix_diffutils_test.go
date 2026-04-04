package testutil

import "testing"

func TestResolveNixDiff(t *testing.T) {
	t.Parallel()

	runResolveNixOracleTests(t, &nixOracleTestConfig{
		name:          "resolveNixDiff",
		envVar:        nixDiffEnv,
		unsetErr:      errNixDiffUnset,
		resolve:       resolveNixDiff,
		version:       pinnedNixDiffVersion,
		wrongVersion:  "diff (GNU diffutils) 3.11",
		successLine:   "diff (GNU diffutils) 3.12",
		binaryName:    "diff",
		requirePrefix: "tests require diffutils ",
	})
}
