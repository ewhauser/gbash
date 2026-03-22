// Package regexponce is a fork of github.com/budougumi0617/regexponce v0.1.1
// with a fix for a nil pointer dereference in the AST visitor.
package regexponce

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/gostaticanalysis/analysisutil"
	"github.com/gostaticanalysis/comment/passes/commentmap"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ssa"
)

const doc = `Below functions should be called at once for performance.
- regexp.Compile
- regexp.MustCompile
- regexp.CompilePOSIX
- regexp.MustCompilePOSIX

Allow call in init and main functions (unless call is in a for loop) because these functions are only called once.
`

// Analyzer checks for correct usage of the regexp package.
var Analyzer = &analysis.Analyzer{
	Name: "regexponce",
	Doc:  doc,
	Run:  run,
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
		commentmap.Analyzer,
	},
}

var _ ast.Visitor = &funcCallVisitor{}

type funcCallVisitor struct {
	usesVarOrCall bool
}

func (v *funcCallVisitor) Visit(node ast.Node) (w ast.Visitor) {
	switch typ := node.(type) {
	case *ast.Ident:
		if typ.Obj != nil && typ.Obj.Kind == ast.Var {
			v.usesVarOrCall = true
		}
	case *ast.CallExpr:
		v.usesVarOrCall = true
	}
	if v.usesVarOrCall {
		return nil
	}
	return v
}

func run(pass *analysis.Pass) (interface{}, error) {
	fs := targetFuncs(pass)
	if len(fs) == 0 {
		return nil, nil
	}

	pass.Report = analysisutil.ReportWithoutIgnore(pass)
	srcFuncs := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA).SrcFuncs

	for _, sf := range srcFuncs {
		if strings.HasPrefix(sf.Name(), "init#") {
			continue
		}

		for _, b := range sf.Blocks {
			var skipped bool
			if strings.HasPrefix(sf.Name(), "main") {
				skipped = true
			}

			if skipped && inFor(b) {
				skipped = false
			}

			if skipped {
				continue
			}

			for _, instr := range b.Instrs {
				for _, f := range fs {
					if !analysisutil.Called(instr, nil, f) {
						continue
					}

					instrTokenPos := instr.Pos()
					if gotPath, _ := astutil.PathEnclosingInterval(fileForPos(pass.Files, instrTokenPos), instrTokenPos, instrTokenPos); len(gotPath) > 0 {
						if callExpr, ok := gotPath[0].(*ast.CallExpr); ok && variablesOrCallInCallExpr(callExpr) {
							continue
						}
					}

					pass.Reportf(instrTokenPos, "%s must be called only once at initialize", f.FullName())
				}
			}
		}
	}

	return nil, nil
}

func variablesOrCallInCallExpr(callExpr *ast.CallExpr) bool {
	if len(callExpr.Args) == 0 {
		return false
	}
	visitor := &funcCallVisitor{}
	ast.Walk(visitor, callExpr.Args[0])
	return visitor.usesVarOrCall
}

func fileForPos(files []*ast.File, pos token.Pos) *ast.File {
	for _, file := range files {
		if pos >= file.Pos() && pos <= file.End() {
			return file
		}
	}
	return nil
}

func inFor(b *ssa.BasicBlock) bool {
	p := b

	for {
		if p.Comment == "for.body" {
			return true
		}

		p = p.Idom()
		if p == nil {
			break
		}
	}

	return false
}

func targetFuncs(pass *analysis.Pass) []*types.Func {
	fs := make([]*types.Func, 0, 4)
	path := "regexp"
	fns := []string{"MustCompile", "Compile", "MustCompilePOSIX", "CompilePOSIX"}

	imports := pass.Pkg.Imports()
	for i := range imports {
		if path == analysisutil.RemoveVendor(imports[i].Path()) {
			for _, fn := range fns {
				obj := imports[i].Scope().Lookup(fn)
				if obj == nil {
					continue
				}

				if f, ok := obj.(*types.Func); ok {
					fs = append(fs, f)
				}
			}
		}
	}

	return fs
}
