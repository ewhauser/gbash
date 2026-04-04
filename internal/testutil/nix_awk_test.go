package testutil

import "testing"

func TestResolveNixAWK(t *testing.T) {
	t.Parallel()

	runResolveNixOracleTests(t, &nixOracleTestConfig{
		name:          "resolveNixAWK",
		envVar:        nixAWKEnv,
		unsetErr:      errNixAWKUnset,
		resolve:       resolveNixAWK,
		version:       pinnedNixAWKVersion,
		wrongVersion:  "GNU Awk 5.3.1, API 4.0, PMA Avon 8-g1",
		successLine:   "GNU Awk 5.3.2, API 4.0, PMA Avon 8-g1",
		binaryName:    "awk",
		requirePrefix: "tests require awk ",
	})
}
