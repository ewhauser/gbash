package expand

import (
	"strings"
	"testing"

	"github.com/ewhauser/gbash/shell/syntax"
)

// These cases used to recurse until the Go runtime killed the process with
// "fatal error: stack overflow", which recover() cannot catch.
func TestArithmSelfReferenceFailsWithRecursionLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		env     testEnv
		script  string
		wantErr string
	}{
		{
			name:    "scalar subscript of itself",
			env:     testEnv{"x": {Set: true, Kind: String, Str: "x[x]"}},
			script:  "echo $((x))",
			wantErr: `x[x]: expression recursion level exceeded (error token is "x[x]")`,
		},
		{
			name: "array subscript through index variable",
			env: testEnv{
				"a": {Set: true, Kind: Indexed},
				"i": {Set: true, Kind: String, Str: "a[i]"},
			},
			script:  "echo ${a[i]}",
			wantErr: `a[i]: expression recursion level exceeded (error token is "a[i]")`,
		},
		{
			name: "array element subscript of itself",
			env: testEnv{
				"a": {Set: true, Kind: Indexed, List: []string{"1"}, Indices: []int{0}},
				"x": {Set: true, Kind: String, Str: "a[x]"},
			},
			script:  "echo $((x))",
			wantErr: `a[x]: expression recursion level exceeded (error token is "a[x]")`,
		},
		{
			name:    "assignment right-hand side of itself",
			env:     testEnv{"x": {Set: true, Kind: String, Str: "y=x"}},
			script:  "echo $((x))",
			wantErr: `y=x: expression recursion level exceeded (error token is "y=x")`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := syntax.NewParser().Parse(strings.NewReader(tc.script), "")
			if err != nil {
				t.Fatal(err)
			}
			cfg := &Config{Env: tc.env}
			call := file.Stmts[0].Cmd.(*syntax.CallExpr)
			_, err = Fields(cfg, call.Args[1])
			if err == nil {
				t.Fatalf("Fields(%q) error = nil, want recursion error", tc.script)
			}
			if got := err.Error(); got != tc.wantErr {
				t.Fatalf("Fields(%q) error = %q, want %q", tc.script, got, tc.wantErr)
			}
			if cfg.arithDepth != 0 {
				t.Fatalf("arithDepth = %d after evaluation, want 0", cfg.arithDepth)
			}
		})
	}
}
