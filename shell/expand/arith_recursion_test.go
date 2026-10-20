package expand

import (
	"strings"
	"testing"
	"time"

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
			name:    "value that re-parses into itself plus one",
			env:     testEnv{"x": {Set: true, Kind: String, Str: "x+1"}},
			script:  "echo $((x))",
			wantErr: `x+1: expression recursion level exceeded (error token is "x+1")`,
		},
		{
			name: "array element that re-parses into itself plus one",
			env: testEnv{
				"A": {Set: true, Kind: Indexed, List: []string{"A[x]+1"}, Indices: []int{0}},
			},
			script:  "echo $((A[x]))",
			wantErr: `A[x]+1: expression recursion level exceeded (error token is "A[x]+1")`,
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

// Each nested $(( )) used to be expanded twice per level (once to re-parse
// the expanded text, once to evaluate it), so 22 levels took over 30 seconds.
func TestArithmNestedSubstitutionIsLinear(t *testing.T) {
	t.Parallel()
	src := "1"
	for range 200 {
		src = "$(( " + src + " ))"
	}
	file, err := syntax.NewParser().Parse(strings.NewReader("echo "+src), "")
	if err != nil {
		t.Fatal(err)
	}
	call := file.Stmts[0].Cmd.(*syntax.CallExpr)
	done := make(chan struct{})
	var fields []string
	go func() {
		defer close(done)
		fields, err = Fields(&Config{Env: testEnv{}}, call.Args[1])
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("nested arithmetic expansion did not finish")
	}
	if err != nil || len(fields) != 1 || fields[0] != "1" {
		t.Fatalf("Fields = %q, %v; want [1]", fields, err)
	}
}
