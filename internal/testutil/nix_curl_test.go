package testutil

import "testing"

func TestResolveNixCurl(t *testing.T) {
	t.Parallel()

	runResolveNixOracleTests(t, &nixOracleTestConfig{
		name:          "resolveNixCurl",
		envVar:        nixCurlEnv,
		unsetErr:      errNixCurlUnset,
		resolve:       resolveNixCurl,
		version:       pinnedNixCurlVersion,
		wrongVersion:  "curl 8.17.1 (x86_64-unknown-linux-gnu)",
		successLine:   "curl 8.18.0 (aarch64-apple-darwin24.0) libcurl/8.18.0",
		binaryName:    "curl",
		requirePrefix: "tests require curl ",
	})
}
