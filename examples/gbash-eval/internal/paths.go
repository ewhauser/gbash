package gbasheval

import (
	"path/filepath"
	"runtime"
)

// ExampleDir returns the source directory for the gbash-eval example.
func ExampleDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("examples", "gbash-eval")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

// DefaultOutputDir returns the default report output directory.
func DefaultOutputDir() string {
	return filepath.Join(ExampleDir(), "results")
}

// DefaultDataDir returns the vendored dataset directory.
func DefaultDataDir() string {
	return filepath.Join(ExampleDir(), "data")
}
