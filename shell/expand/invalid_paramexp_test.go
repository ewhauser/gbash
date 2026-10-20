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

// The parser accepts trailing text after an @ operator, as in ${x@Q"abc"};
// expansion used to panic on the unknown operator.
func TestUnknownAtOperatorReportsBadSubstitution(t *testing.T) {
	t.Parallel()
	env := testEnv{"x": {Set: true, Kind: String, Str: "abc"}}
	_, err := Fields(&Config{Env: env}, parseCommandWord(t, `${x@Q"abc"}`))
	if err == nil || err.Error() != `${x@Q"abc"}: bad substitution` {
		t.Fatalf("Fields error = %v, want bad substitution", err)
	}
}
