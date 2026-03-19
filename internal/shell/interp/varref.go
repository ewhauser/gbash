package interp

import (
	"bytes"
	"maps"
	"slices"
	"strings"

	"github.com/ewhauser/gbash/internal/shell/expand"
	"github.com/ewhauser/gbash/internal/shell/syntax"
)

func parseVarRef(src string) (*syntax.VarRef, error) {
	p := syntax.NewParser()
	return p.VarRef(strings.NewReader(src))
}

func literalSubscript(kind syntax.SubscriptKind, lit string) *syntax.Subscript {
	return &syntax.Subscript{
		Kind: kind,
		Expr: &syntax.Word{Parts: []syntax.WordPart{
			&syntax.Lit{Value: lit},
		}},
	}
}

func subscriptLit(sub *syntax.Subscript) string {
	if sub == nil {
		return ""
	}
	switch sub.Kind {
	case syntax.SubscriptAt:
		return "@"
	case syntax.SubscriptStar:
		return "*"
	default:
		word, ok := sub.Expr.(*syntax.Word)
		if !ok {
			return ""
		}
		return word.Lit()
	}
}

func subscriptWord(sub *syntax.Subscript) (*syntax.Word, bool) {
	if sub == nil {
		return nil, false
	}
	word, ok := sub.Expr.(*syntax.Word)
	return word, ok
}

func printVarRef(ref *syntax.VarRef) string {
	if ref == nil {
		return ""
	}
	var buf bytes.Buffer
	printer := syntax.NewPrinter()
	if err := printer.Print(&buf, ref); err != nil {
		return ref.Name.Value
	}
	return buf.String()
}

func (r *Runner) resolveVarRef(ref *syntax.VarRef) (*syntax.VarRef, expand.Variable, error) {
	vr := r.lookupVar(ref.Name.Value)
	return vr.ResolveRef(r.writeEnv, ref)
}

func (r *Runner) strictVarRef(src string) (*syntax.VarRef, error) {
	return parseVarRef(src)
}

func (r *Runner) looseVarRef(src string) *syntax.VarRef {
	ref, err := r.strictVarRef(src)
	if err == nil {
		return ref
	}
	return &syntax.VarRef{Name: &syntax.Lit{Value: src}}
}

func (r *Runner) looseVarRefWord(word *syntax.Word) *syntax.VarRef {
	var buf bytes.Buffer
	printer := syntax.NewPrinter()
	if err := printer.Print(&buf, word); err == nil {
		if ref, err := r.strictVarRef(buf.String()); err == nil {
			return ref
		}
	}
	return &syntax.VarRef{Name: &syntax.Lit{Value: r.literal(word)}}
}

func (r *Runner) refIsSet(ref *syntax.VarRef) bool {
	ref, vr, err := r.resolveVarRef(ref)
	if err != nil {
		return false
	}
	if ref == nil || ref.Index == nil {
		return vr.IsSet()
	}
	switch vr.Kind {
	case expand.String:
		return vr.IsSet() && r.arithm(ref.Index.Expr) == 0
	case expand.Indexed:
		index := r.arithm(ref.Index.Expr)
		if index < 0 {
			index = len(vr.List) + index
		}
		return index >= 0 && index < len(vr.List)
	case expand.Associative:
		word, ok := subscriptWord(ref.Index)
		if !ok {
			return false
		}
		_, ok = vr.Map[r.literal(word)]
		return ok
	default:
		return false
	}
}

func (r *Runner) refIsNameRef(ref *syntax.VarRef) bool {
	return ref != nil && ref.Index == nil && r.lookupVar(ref.Name.Value).Kind == expand.NameRef
}

func (r *Runner) setVarByRef(prev expand.Variable, ref *syntax.VarRef, vr expand.Variable) error {
	ref, prev, err := prev.ResolveRef(r.writeEnv, ref)
	if err != nil {
		return err
	}
	prev.Set = true
	name := ref.Name.Value
	index := ref.Index

	if vr.Kind == expand.String && index == nil {
		// When assigning a string to an array, fall back to the
		// zero value for the index.
		switch prev.Kind {
		case expand.Indexed:
			index = literalSubscript(syntax.SubscriptExpr, "0")
		case expand.Associative:
			index = &syntax.Subscript{
				Kind: syntax.SubscriptExpr,
				Expr: &syntax.Word{Parts: []syntax.WordPart{
					&syntax.DblQuoted{},
				}},
			}
		}
	}
	if index == nil {
		r.setVar(name, vr)
		return nil
	}

	valStr := vr.Str

	var list []string
	switch prev.Kind {
	case expand.String:
		list = append(list, prev.Str)
	case expand.Indexed:
		list = slices.Clone(prev.List)
	case expand.Associative:
		word, ok := subscriptWord(index)
		if !ok {
			return nil
		}
		key := r.literal(word)
		prev.Map = maps.Clone(prev.Map)
		if prev.Map == nil {
			prev.Map = make(map[string]string)
		}
		prev.Map[key] = valStr
		r.setVar(name, prev)
		return nil
	}
	key := r.arithm(index.Expr)
	for len(list) < key+1 {
		list = append(list, "")
	}
	list[key] = valStr
	prev.Kind = expand.Indexed
	prev.List = list
	r.setVar(name, prev)
	return nil
}
