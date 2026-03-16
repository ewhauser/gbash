package conformance

import "testing"

func TestConformance(t *testing.T) {
	suites := []SuiteConfig{
		{
			Name:         "bash",
			SpecDir:      "oils",
			BinDir:       "bin",
			ManifestPath: "manifest.json",
			OracleMode:   OracleBash,
		},
		{
			Name:         "posix",
			SpecDir:      "oils",
			SpecFiles:    []string{"posix.test.sh"},
			BinDir:       "bin",
			FixtureDirs:  []string{"fixtures"},
			ManifestPath: "manifest.json",
			OracleMode:   OracleBashPosix,
		},
	}

	for _, cfg := range suites {
		t.Run(cfg.Name, func(t *testing.T) {
			t.Parallel()
			RunSuite(t, &cfg)
		})
	}
}
