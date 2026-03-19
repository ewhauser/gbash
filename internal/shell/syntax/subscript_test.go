package syntax

import (
	"strings"
	"testing"
)

func TestVarRefSubscriptKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src  string
		kind SubscriptKind
		mode SubscriptMode
	}{
		{src: "a[1]", kind: SubscriptExpr, mode: SubscriptAuto},
		{src: "a[@]", kind: SubscriptAt, mode: SubscriptAuto},
		{src: "a[*]", kind: SubscriptStar, mode: SubscriptAuto},
	}

	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			ref, err := NewParser().VarRef(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("VarRef(%q) error = %v", tc.src, err)
			}
			if ref.Index == nil {
				t.Fatalf("VarRef(%q) index = nil", tc.src)
			}
			if ref.Index.Kind != tc.kind {
				t.Fatalf("VarRef(%q) kind = %v, want %v", tc.src, ref.Index.Kind, tc.kind)
			}
			if ref.Index.Mode != tc.mode {
				t.Fatalf("VarRef(%q) mode = %v, want %v", tc.src, ref.Index.Mode, tc.mode)
			}
		})
	}
}

func TestParseSubscriptKinds(t *testing.T) {
	t.Parallel()

	file, err := NewParser().Parse(strings.NewReader("echo ${foo[@]} ${foo[*]} ${foo[1]}\ndeclare foo[*]\n"), "")
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	call := file.Stmts[0].Cmd.(*CallExpr)
	indexes := []SubscriptKind{
		call.Args[1].Parts[0].(*ParamExp).Index.Kind,
		call.Args[2].Parts[0].(*ParamExp).Index.Kind,
		call.Args[3].Parts[0].(*ParamExp).Index.Kind,
	}
	want := []SubscriptKind{SubscriptAt, SubscriptStar, SubscriptExpr}
	for i, got := range indexes {
		if got != want[i] {
			t.Fatalf("call arg %d kind = %v, want %v", i+1, got, want[i])
		}
	}

	decl := file.Stmts[1].Cmd.(*DeclClause)
	name, ok := decl.Operands[0].(*DeclName)
	if !ok {
		t.Fatalf("declare operand = %T, want *DeclName", decl.Operands[0])
	}
	if got := name.Ref.Index.Kind; got != SubscriptStar {
		t.Fatalf("declare subscript kind = %v, want %v", got, SubscriptStar)
	}
}

func TestParseSubscriptModesAndContexts(t *testing.T) {
	t.Parallel()

	src := strings.Join([]string{
		"declare -A foo=([a]=b)",
		"declare -A foo[a]=",
		"declare -a bar[1]=",
		"[[ -v assoc[$key] ]]",
		"[[ -R ref ]]",
	}, "\n")
	file, err := NewParser().Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	declAssoc := file.Stmts[0].Cmd.(*DeclClause)
	as0 := declAssoc.Operands[1].(*DeclAssign).Assign
	if got := as0.Array.Elems[0].Index.Mode; got != SubscriptAssociative {
		t.Fatalf("declare -A array elem mode = %v, want %v", got, SubscriptAssociative)
	}

	declAssocRef := file.Stmts[1].Cmd.(*DeclClause)
	as1 := declAssocRef.Operands[1].(*DeclAssign).Assign
	if got := as1.Ref.Index.Mode; got != SubscriptAssociative {
		t.Fatalf("declare -A ref mode = %v, want %v", got, SubscriptAssociative)
	}

	declIndexedRef := file.Stmts[2].Cmd.(*DeclClause)
	as2 := declIndexedRef.Operands[1].(*DeclAssign).Assign
	if got := as2.Ref.Index.Mode; got != SubscriptIndexed {
		t.Fatalf("declare -a ref mode = %v, want %v", got, SubscriptIndexed)
	}

	testVarSet := file.Stmts[3].Cmd.(*TestClause)
	ref := testVarSet.X.(*CondUnary).X.(*CondVarRef).Ref
	if got := ref.Context; got != VarRefVarSet {
		t.Fatalf("[[ -v ]] ref context = %v, want %v", got, VarRefVarSet)
	}

	testRefVar := file.Stmts[4].Cmd.(*TestClause)
	ref = testRefVar.X.(*CondUnary).X.(*CondVarRef).Ref
	if got := ref.Context; got != VarRefDefault {
		t.Fatalf("[[ -R ]] ref context = %v, want %v", got, VarRefDefault)
	}
}
