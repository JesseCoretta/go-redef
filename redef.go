package redef

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name: "redef",
	Doc:  "reports unnecessary variable redefinitions (shadowing) within a function",
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
	Run: run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return
		}

		// Only consider :=
		if as.Tok != token.DEFINE {
			return
		}

		// For each LHS identifier, check if it shadows an outer variable.
		for _, lhs := range as.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name == "_" {
				continue
			}

			obj := pass.TypesInfo.Defs[ident]
			if obj == nil {
				continue
			}

			// Find the outer object with the same name.
			outer := findOuter(pass.TypesInfo, ident, obj)
			if outer == nil {
				continue
			}

			// Report all shadowing for now.
			pass.Reportf(ident.Pos(),
				"variable %q is redefined and shadows an outer %q",
				ident.Name, ident.Name)
		}
	})

	return nil, nil
}

// findOuter returns the nearest outer object with the same name.
func findOuter(info *types.Info, ident *ast.Ident, inner types.Object) types.Object {
	scope := inner.Parent()
	if scope == nil {
		return nil
	}

	// Walk outward through parent scopes.
	for s := scope.Parent(); s != nil; s = s.Parent() {
		if obj := s.Lookup(ident.Name); obj != nil {
			return obj
		}
	}
	return nil
}
