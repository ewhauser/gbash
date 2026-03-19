package expand

import (
	"strings"
	"testing"

	"github.com/ewhauser/gbash/internal/shell/syntax"
)

func parseVarRefForTest(t *testing.T, src string) *syntax.VarRef {
	t.Helper()
	ref, err := syntax.NewParser().VarRef(strings.NewReader(src))
	if err != nil {
		t.Fatalf("VarRef(%q) error = %v", src, err)
	}
	return ref
}

func TestResolveRef(t *testing.T) {
	t.Parallel()

	env := testEnv{
		"arr": {
			Set:  true,
			Kind: Indexed,
			List: []string{"x", "y"},
		},
		"whole": {
			Set:  true,
			Kind: NameRef,
			Str:  "arr",
		},
		"elem": {
			Set:  true,
			Kind: NameRef,
			Str:  "arr[0]",
		},
	}

	ref, vr, err := env.Get("whole").ResolveRef(env, parseVarRefForTest(t, "whole[1]"))
	if err != nil {
		t.Fatalf("ResolveRef whole error = %v", err)
	}
	if got := ref.Name.Value; got != "arr" {
		t.Fatalf("resolved name = %q, want arr", got)
	}
	if got := subscriptLit(ref.Index); got != "1" {
		t.Fatalf("resolved index = %q, want 1", got)
	}
	if vr.Kind != Indexed {
		t.Fatalf("resolved kind = %v, want Indexed", vr.Kind)
	}

	_, _, err = env.Get("elem").ResolveRef(env, parseVarRefForTest(t, "elem[1]"))
	if err == nil {
		t.Fatal("ResolveRef elem[1] succeeded, want error")
	}
	if _, ok := err.(InvalidIdentifierError); !ok {
		t.Fatalf("ResolveRef elem[1] error = %T, want InvalidIdentifierError", err)
	}
}
