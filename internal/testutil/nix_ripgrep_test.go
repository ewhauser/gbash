package testutil

import "testing"

func TestResolveNixRipgrep(t *testing.T) {
	t.Parallel()

	runResolveNixOracleTests(t, &nixOracleTestConfig{
		name:          "resolveNixRipgrep",
		envVar:        nixRipgrepEnv,
		unsetErr:      errNixRipgrepUnset,
		resolve:       resolveNixRipgrep,
		version:       pinnedNixRipgrepVersion,
		wrongVersion:  "ripgrep 14.1.1",
		successLine:   "ripgrep 15.1.0",
		binaryName:    "rg",
		requirePrefix: "tests require ripgrep ",
	})
}
