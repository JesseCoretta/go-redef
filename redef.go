package redef

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name: "redef",
	Doc:  "reports unnecessary variable redefinitions (shadowing) within a Go function or method",
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
	Run: run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Build parent map
	parent := make(map[ast.Node]ast.Node)
	insp.Nodes(nil, func(n ast.Node, push bool) bool {
		if !push {
			return true
		}
		ast.Inspect(n, func(child ast.Node) bool {
			if child == nil || child == n {
				return true
			}
			if _, exists := parent[child]; !exists {
				parent[child] = n
			}
			return true
		})
		return true
	})

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		pos := pass.Fset.Position(n.Pos())
		if ignoreTests && strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE {
			return
		}

		for _, lhs := range as.Lhs {

			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name == "_" {
				continue
			}

			obj := pass.TypesInfo.Defs[ident]
			if obj == nil {
				continue
			}

			outer := findOuter(pass.TypesInfo, ident, obj)
			if outer == nil {
				continue
			}

			stmt := findOwningStmt(as, parent)
			if stmt == nil {
				pass.Reportf(ident.Pos(),
					"variable %q is redefined and shadows an outer %q",
					ident.Name, ident.Name)
				continue
			}

			block := findEnclosingBlock(stmt, parent)
			if block == nil {
				pass.Reportf(ident.Pos(),
					"variable %q is redefined and shadows an outer %q",
					ident.Name, ident.Name)
				continue
			}

			if allowShortIf {
				if _, ok := parent[as].(*ast.IfStmt); ok {
					return
				}
			}

			if allowSameLine {
				if pass.Fset.Position(ident.Pos()).Line == pass.Fset.Position(outer.Pos()).Line {
					return
				}
			}

			if allowLoopShadow {
				if _, ok := parent[stmt].(*ast.ForStmt); ok {
					return
				}
				if _, ok := parent[stmt].(*ast.RangeStmt); ok {
					return
				}
			}

			if allowDeadOuter {
				if !outerUsedLater(outer, stmt, block, pass.TypesInfo) {
					return
				}
			}

			if allowErrShadow {
				if ident.Name == "err" && outer.Name() == "err" {
					return
				}
			}

			if allowGuardShadow {
				if isGuardClauseOnly(outer, stmt, block, pass.TypesInfo) {
					return
				}
			}

			if allowTableTests {
				if isTableTestPattern(as, parent, pass.TypesInfo) {
					return
				}
			}

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

	for s := scope.Parent(); s != nil; s = s.Parent() {
		if obj := s.Lookup(ident.Name); obj != nil {
			return obj
		}
	}
	return nil
}

// findOwningStmt walks upward using the parent map until it finds an ast.Stmt.
func findOwningStmt(n ast.Node, parent map[ast.Node]ast.Node) ast.Stmt {
	for cur := n; cur != nil; cur = parent[cur] {
		if s, ok := cur.(ast.Stmt); ok {
			return s
		}
	}
	return nil
}

// findEnclosingBlock walks upward until it finds the nearest *ast.BlockStmt.
func findEnclosingBlock(n ast.Node, parent map[ast.Node]ast.Node) *ast.BlockStmt {
	for cur := n; cur != nil; cur = parent[cur] {
		if b, ok := cur.(*ast.BlockStmt); ok {
			return b
		}
	}
	return nil
}

// outerUsedLater reports whether the OUTER object is used in any statement
// after stmt within block.
func outerUsedLater(outer types.Object, stmt ast.Stmt, block *ast.BlockStmt, info *types.Info) bool {
	if block == nil {
		return false
	}

	idx := -1
	for i, s := range block.List {
		if s == stmt {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false
	}

	for _, later := range block.List[idx+1:] {
		found := false
		ast.Inspect(later, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && info.Uses[id] == outer {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}

	return false
}

func isTableTestPattern(as *ast.AssignStmt, parent map[ast.Node]ast.Node, info *types.Info) bool {
	// Must be a := with exactly one LHS and one RHS
	if len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return false
	}

	identLHS, ok := as.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	identRHS, ok := as.Rhs[0].(*ast.Ident)
	if !ok {
		return false
	}

	// Must be inside a RangeStmt
	rng, ok := parent[as].(*ast.RangeStmt)
	if !ok {
		return false
	}

	// Range must bind an identifier (e.g., "tt")
	rangeIdent, ok := rng.Value.(*ast.Ident)
	if !ok {
		return false
	}

	// LHS must match the range variable name
	if identLHS.Name != rangeIdent.Name {
		return false
	}

	// RHS must match the range variable name
	if identRHS.Name != rangeIdent.Name {
		return false
	}

	// Must refer to the same object
	objRange := info.Defs[rangeIdent]
	objLHS := info.Defs[identLHS]
	objRHS := info.Uses[identRHS]

	if objRange == nil || objLHS == nil || objRHS == nil {
		return false
	}

	// Pattern is: tt := tt inside a range over tests
	return objRange == objRHS
}

func isGuardClauseOnly(outer types.Object, stmt ast.Stmt, block *ast.BlockStmt, info *types.Info) bool {
	if block == nil {
		return false
	}

	// Scan all statements BEFORE the shadowing stmt
	for _, s := range block.List {
		if s == stmt {
			break
		}

		// Look for uses of outer
		used := false
		ast.Inspect(s, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && info.Uses[id] == outer {
				used = true
				return false
			}
			return true
		})

		if !used {
			continue
		}

		// If used, it must be inside an if-statement guard
		if ifs, ok := s.(*ast.IfStmt); ok {
			// Condition must reference outer
			condUsesOuter := false
			ast.Inspect(ifs.Cond, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if ok && info.Uses[id] == outer {
					condUsesOuter = true
					return false
				}
				return true
			})

			if !condUsesOuter {
				return false
			}

			// Body must immediately return/break/continue
			if len(ifs.Body.List) == 0 {
				return false
			}

			switch ifs.Body.List[0].(type) {
			case *ast.ReturnStmt, *ast.BranchStmt:
				continue
			default:
				return false
			}
		} else {
			// Used outside an if-guard; not a guard-only pattern
			return false
		}
	}

	return true
}

// flag vars
var (
	ignoreTests,
	allowShortIf,
	allowSameLine,
	allowDeadOuter,
	allowErrShadow,
	allowLoopShadow,
	allowTableTests,
	allowGuardShadow bool
)

func init() {
	Analyzer.Flags.BoolVar(&allowErrShadow, "allow-err-shadow", false,
		"Allow shadowing when both inner and outer variables are named err")
	Analyzer.Flags.BoolVar(&allowGuardShadow, "allow-guard-shadow", false,
		"Allow shadowing when the outer variable is only used in guard clauses")
	Analyzer.Flags.BoolVar(&ignoreTests, "ignore-tests", false,
		"Avoid checking any _test.go files")
	Analyzer.Flags.BoolVar(&allowDeadOuter, "allow-dead-outer", false,
		"Allow shadowing when the outer variable is never used again")
	Analyzer.Flags.BoolVar(&allowShortIf, "allow-short-if", false,
		"Allow shadowing inside short-if statements")
	Analyzer.Flags.BoolVar(&allowSameLine, "allow-same-line", false,
		"Allow shadowing when inner and outer appear on the same line")
	Analyzer.Flags.BoolVar(&allowLoopShadow, "allow-loop-shadow", false,
		"Allow shadowing inside for/range loops")
	Analyzer.Flags.BoolVar(&allowTableTests, "allow-table-tests", false,
		"Allow shadowing in table-driven tests")
}
