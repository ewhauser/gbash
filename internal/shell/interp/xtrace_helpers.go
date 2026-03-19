package interp

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ewhauser/gbash/internal/shell/expand"
	"github.com/ewhauser/gbash/internal/shell/syntax"
)

func printSyntaxNode(node syntax.Node) string {
	if node == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := syntax.NewPrinter().Print(&buf, node); err != nil {
		panic(err)
	}
	return buf.String()
}

func quoteTraceValue(value string) string {
	quoted, err := syntax.Quote(value, syntax.LangBash)
	if err != nil {
		panic(err)
	}
	return quoted
}

func quoteTraceArrayValue(value string) string {
	quoted := quoteTraceValue(value)
	if quoted == value {
		return "'" + value + "'"
	}
	return quoted
}

func traceAssignFieldRaw(ref *syntax.VarRef, vr expand.Variable, appendValue bool) string {
	op := "="
	if appendValue {
		op = "+="
	}
	return printVarRef(ref) + op + vr.String()
}

func (r *Runner) traceAssignString(ref *syntax.VarRef, vr expand.Variable, appendValue bool) string {
	op := "="
	if appendValue {
		op = "+="
	}
	return printVarRef(ref) + op + quoteTraceValue(vr.String())
}

func (r *Runner) traceArrayAssign(as *syntax.Assign) string {
	var b strings.Builder
	b.WriteString(printVarRef(as.Ref))
	if as.Append {
		b.WriteByte('+')
	}
	b.WriteString("=(")
	for i, elem := range as.Array.Elems {
		if i > 0 {
			b.WriteByte(' ')
		}
		if elem.Index != nil {
			b.WriteString(printSyntaxNode(elem.Index))
			if elem.Kind == syntax.ArrayElemKeyedAppend {
				b.WriteString("+=")
			} else {
				b.WriteByte('=')
			}
		}
		if elem.Value != nil {
			b.WriteString(quoteTraceArrayValue(r.literal(elem.Value)))
		}
	}
	b.WriteByte(')')
	return b.String()
}

func (r *Runner) traceCondExpr(expr syntax.CondExpr, trace *tracer) string {
	switch x := expr.(type) {
	case *syntax.CondWord:
		return trace.traceArg(r.literal(x.Word))
	case *syntax.CondPattern:
		return r.pattern(x.Pattern)
	case *syntax.CondRegex:
		return trace.traceArg(r.literal(x.Word))
	case *syntax.CondVarRef:
		return printVarRef(x.Ref)
	case *syntax.CondParen:
		return "( " + r.traceCondExpr(x.X, trace) + " )"
	case *syntax.CondBinary:
		return strings.TrimSpace(fmt.Sprintf("%s %s %s", r.traceCondExpr(x.X, trace), x.Op, r.traceCondExpr(x.Y, trace)))
	case *syntax.CondUnary:
		return strings.TrimSpace(fmt.Sprintf("%s %s", x.Op, r.traceCondExpr(x.X, trace)))
	default:
		return printSyntaxNode(expr.(syntax.Node))
	}
}
