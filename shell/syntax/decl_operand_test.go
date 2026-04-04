package syntax

import (
	"reflect"
	"strings"
	"testing"
)

func TestParserDeclOperand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src      string
		wantType any
		want     string
	}{
		{src: "-a", wantType: &DeclFlag{}, want: "-a"},
		{src: "foo", wantType: &DeclName{}, want: "foo"},
		{src: "foo=bar", wantType: &DeclAssign{}, want: "foo=bar"},
		{src: "foo=(1 2)", wantType: &DeclAssign{}, want: "foo=(1 2)"},
		{src: "foo=([k]=v [k]+=x)", wantType: &DeclAssign{}, want: "foo=([k]=v [k]+=x)"},
		{src: "foo[$k]=bar", wantType: &DeclAssign{}, want: "foo[$k]=bar"},
		{src: "$x", wantType: &DeclDynamicWord{}, want: "$x"},
	}

	assertDeclOperandCases(t, tests, func(p *Parser, src string) (DeclOperand, error) {
		return p.DeclOperand(strings.NewReader(src))
	})
}

func TestParserDeclOperandField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		src      string
		wantType any
		want     string
	}{
		{src: "foo=1 2", wantType: &DeclAssign{}, want: "foo=1 2"},
		{src: "foo=$HOME", wantType: &DeclAssign{}, want: "foo=$HOME"},
		{src: `foo="1 2"`, wantType: &DeclAssign{}, want: `foo="1 2"`},
		{src: `arr=("$HOME" $(printf hacked) plain)`, wantType: &DeclAssign{}, want: `arr=("$HOME" $(printf hacked) plain)`},
		{src: `arr=([k]=v [k]+=x)`, wantType: &DeclAssign{}, want: `arr=([k]=v [k]+=x)`},
		{src: `-@(*.py)`, wantType: &DeclFlag{}, want: `-@(*.py)`},
		{src: "$x", wantType: &DeclDynamicWord{}, want: "$x"},
	}

	assertDeclOperandCases(t, tests, func(p *Parser, src string) (DeclOperand, error) {
		return p.DeclOperandField(strings.NewReader(src))
	})
}

func assertDeclOperandCases(t *testing.T, tests []struct {
	src      string
	wantType any
	want     string
}, parse func(*Parser, string) (DeclOperand, error)) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			p := NewParser(Variant(LangBash))
			op, err := parse(p, tc.src)
			if err != nil {
				t.Fatalf("parse(%q) error = %v", tc.src, err)
			}
			if reflect.TypeOf(op) != reflect.TypeOf(tc.wantType) {
				t.Fatalf("parse(%q) type = %T, want %T", tc.src, op, tc.wantType)
			}
			var sb strings.Builder
			if err := NewPrinter().Print(&sb, op); err != nil {
				t.Fatalf("Print(%q) error = %v", tc.src, err)
			}
			if got := sb.String(); got != tc.want {
				t.Fatalf("parse(%q) printed %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
