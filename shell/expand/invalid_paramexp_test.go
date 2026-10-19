package expand

import "testing"

// The parser keeps ${%} as a bad substitution with a nil Param. Inside
// double quotes, the array-field fast path dereferenced that Param and
// panicked instead of reporting bash's error.
func TestInvalidParamExpReportsBadSubstitution(t *testing.T) {
	t.Parallel()
	for _, src := range []string{`${%}`, `"${%}"`, `"a${%}b"`} {
		_, err := Fields(&Config{Env: testEnv{}}, parseCommandWord(t, src))
		if err == nil || err.Error() != "${%}: bad substitution" {
			t.Errorf("Fields(%s) error = %v, want %q", src, err, "${%}: bad substitution")
		}
	}
}
