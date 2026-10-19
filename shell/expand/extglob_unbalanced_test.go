package expand

import (
	"io/fs"
	"reflect"
	"testing"
)

// With extglob on, a pattern such as "*(" taken from a variable used to
// compile to an invalid regular expression, and regexp.MustCompile panicked,
// aborting the whole execution. Bash matches the unterminated group literally.
func TestUnbalancedExtGlobFromVariableIsLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src  string
		want []string
	}{
		{src: `$p`, want: []string{"*("}},
		{src: `${v#$p}`, want: []string{"a*(b"}},
		{src: `${v#a$p}`, want: []string{"b"}},
		{src: `${v%$p*}`, want: []string{"a*(b"}},
		{src: `${v//$p/X}`, want: []string{"aXb"}},
		{src: `${v^^$p}`, want: []string{"a*(b"}},
		{src: `${v,,$p}`, want: []string{"a*(b"}},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{
				Env: testEnv{
					"p": {Set: true, Kind: String, Str: "*("},
					"v": {Set: true, Kind: String, Str: "a*(b"},
				},
				ExtGlob: true,
				ReadDir: func(string) ([]fs.DirEntry, error) {
					return []fs.DirEntry{&mockFileInfo{name: "a("}, &mockFileInfo{name: "b"}}, nil
				},
			}
			got, err := Fields(cfg, parseCommandWord(t, tc.src))
			if err != nil {
				t.Fatalf("Fields(%s) error = %v", tc.src, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Fields(%s) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
