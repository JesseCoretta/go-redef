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
	parent := buildParentMap(insp)

	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil)}, func(n ast.Node) {
		if skipFile(pass, n) {
			return
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE {
			return
		}
		processAssign(pass, as, parent)
	})

	return nil, nil
}

func buildParentMap(insp *inspector.Inspector) map[ast.Node]ast.Node {
	parent := make(map[ast.Node]ast.Node)

	insp.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
		if push && len(stack) > 1 {
			parent[n] = stack[len(stack)-2]
		}
		return true
	})

	return parent
}

func skipFile(pass *analysis.Pass, n ast.Node) (skip bool) {
	if !ignoreTests {
		pos := pass.Fset.Position(n.Pos())
		skip = strings.HasSuffix(pos.Filename, "_test.go")
	}

	return
}

func processAssign(pass *analysis.Pass, as *ast.AssignStmt, parent map[ast.Node]ast.Node) {
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
		if shouldSkipShadow(pass, ident, outer, as, parent) {
			continue
		}
		pass.Reportf(ident.Pos(),
			"variable %q is redefined and shadows an outer %q",
			ident.Name, ident.Name)
	}
}

func shouldSkipShadow(
	pass *analysis.Pass,
	ident *ast.Ident,
	outer types.Object,
	as *ast.AssignStmt,
	parent map[ast.Node]ast.Node,
) (should bool) {
	block := findEnclosingBlock(as, parent)
	if block == nil {
		return
	}

	stmt := findOwningStmt(as, parent)
	if stmt == nil {
		return
	}

	for _, should = range []bool{
		skipForShortIf(parent, as),
		skipForSameLine(pass, ident, outer),
		skipForLoopShadow(parent, stmt),
		skipForDeadOuter(pass, outer, stmt, block),
		skipForErrShadow(ident, outer),
		skipForGuardShadow(pass, outer, stmt, block),
		skipForTableTests(as, parent, pass.TypesInfo),
	} {
		if should {
			break
		}
	}

	return
}

func skipForShortIf(parent map[ast.Node]ast.Node, as *ast.AssignStmt) bool {
	_, ok := parent[as].(*ast.IfStmt)
	return ok && allowShortIf
}

func skipForSameLine(pass *analysis.Pass, ident *ast.Ident, outer types.Object) bool {
	return pass.Fset.Position(ident.Pos()).Line ==
		pass.Fset.Position(outer.Pos()).Line && allowSameLine
}

func skipForLoopShadow(parent map[ast.Node]ast.Node, stmt ast.Stmt) (ok bool) {
	if allowLoopShadow {
		if _, ok = parent[stmt].(*ast.ForStmt); ok {
			return
		}
		if _, ok = parent[stmt].(*ast.RangeStmt); ok {
			return
		}
	}
	return
}

func skipForDeadOuter(
	pass *analysis.Pass,
	outer types.Object,
	stmt ast.Stmt,
	block *ast.BlockStmt,
) (allow bool) {
	if allowDeadOuter {
		allow = !outerUsedLater(outer, stmt, block, pass.TypesInfo) && allowDeadOuter
	}

	return
}

func skipForErrShadow(ident *ast.Ident, outer types.Object) (allow bool) {
	if allowErrShadow {
		allow = ident.Name == "err" && outer.Name() == "err"
	}
	return
}

func skipForGuardShadow(pass *analysis.Pass, outer types.Object, stmt ast.Stmt, block *ast.BlockStmt) bool {
	return isGuardClauseOnly(outer, stmt, block, pass.TypesInfo) && allowGuardShadow
}

func skipForTableTests(as *ast.AssignStmt, parent map[ast.Node]ast.Node, info *types.Info) bool {
	return isTableTestPattern(as, parent, info) && allowTableTests
}

func findOuter(info *types.Info, ident *ast.Ident, inner types.Object) types.Object {
	name := ident.Name
	scope := inner.Parent()
	if scope == nil {
		return nil
	}

	// Walk outward through lexical scopes
	for s := scope.Parent(); s != nil; s = s.Parent() {
		if obj := s.Lookup(name); obj != nil {
			// Ensure it's a variable, not a func param, not a field, etc.
			if _, ok := obj.(*types.Var); ok {
				return obj
			}
		}
	}

	return nil
}

// findOwningStmt walks upward using the parent map until it finds an ast.Stmt.
func findOwningStmt(n ast.Node, parent map[ast.Node]ast.Node) (s ast.Stmt) {
	for cur := n; cur != nil; cur = parent[cur] {
		var ok bool
		if s, ok = cur.(ast.Stmt); ok {
			break
		}
	}
	return
}

// findEnclosingBlock walks upward until it finds the nearest *ast.BlockStmt.
func findEnclosingBlock(n ast.Node, parent map[ast.Node]ast.Node) (b *ast.BlockStmt) {
	for cur := n; cur != nil; cur = parent[cur] {
		var ok bool
		if b, ok = cur.(*ast.BlockStmt); ok {
			break
		}
	}
	return
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
				id, assert := n.(*ast.Ident)
				if assert && info.Uses[id] == outer {
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
