package mustclose

// AST refinement overlay.
//
// Everything in this file reads the AST and only rewrites diagnostic messages; it
// never adds or removes diagnostics. It is currently disabled: the call to
// refineMessages in run (analyzer.go) is commented out, so all diagnostics use the
// uniform msgNotClosed message. Re-enable by uncommenting that call.
//
// It recovers source-level names that SSA discards by reverse-mapping the
// diagnostic position back to the AST. That reverse mapping is a heuristic (see
// README notes on the trade-off); keeping it isolated here makes it easy to drop.

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/astutil"
)

// refineMessages upgrades the uniform message with source-level detail: the name
// of the callee whose result was discarded ("...on the result of X"), or the name
// of the variable the closer was bound to ("...on x"). SSA loses these names, so
// they are recovered from the AST at the diagnostic position.
func refineMessages(pass *analysis.Pass, diags []analysis.Diagnostic) {
	discarded := discardedCallNames(pass)
	for i := range diags {
		pos := diags[i].Pos
		if name, ok := discarded[pos]; ok {
			diags[i].Message = fmt.Sprintf("Close is not called on the result of %s", name)
			continue
		}
		if name := variableName(pass, pos); name != "" {
			diags[i].Message = fmt.Sprintf("Close is not called on %s", name)
		}
	}
}

// variableName recovers the name of the variable a closer at pos was bound to, by
// locating the enclosing assignment, declaration or type switch in the AST. It
// returns "" when no unambiguous name is available (e.g. a multi-closer
// assignment, or a blank identifier).
func variableName(pass *analysis.Pass, pos token.Pos) string {
	if pos == token.NoPos {
		return ""
	}
	for _, f := range pass.Files {
		if pos < f.Pos() || pos > f.End() {
			continue
		}
		path, _ := astutil.PathEnclosingInterval(f, pos, pos)
		for _, n := range path {
			switch node := n.(type) {
			case *ast.AssignStmt:
				if name := assignName(pass, node, pos); name != "" {
					return name
				}
			case *ast.ValueSpec:
				if name := valueSpecName(pass, node, pos); name != "" {
					return name
				}
			case *ast.TypeSwitchStmt:
				if a, ok := node.Assign.(*ast.AssignStmt); ok && len(a.Lhs) == 1 {
					return identName(a.Lhs[0])
				}
			}
		}
		return ""
	}
	return ""
}

// assignName returns the LHS variable name a closer at pos corresponds to within
// an assignment. For a parallel assignment it matches the LHS or RHS expression
// that contains pos; for a multi-value assignment (one call, several results) it
// returns the sole closer-typed LHS, or "" if ambiguous.
func assignName(pass *analysis.Pass, a *ast.AssignStmt, pos token.Pos) string {
	if len(a.Lhs) == len(a.Rhs) {
		for i := range a.Lhs {
			if within(pos, a.Lhs[i]) || within(pos, a.Rhs[i]) {
				return identName(a.Lhs[i])
			}
		}
		return ""
	}
	for _, l := range a.Lhs {
		if within(pos, l) {
			return identName(l)
		}
	}
	return soleCloserName(pass, a.Lhs)
}

// valueSpecName returns the declared name a closer at pos corresponds to within a
// var declaration.
func valueSpecName(pass *analysis.Pass, vs *ast.ValueSpec, pos token.Pos) string {
	if len(vs.Names) == len(vs.Values) {
		for i := range vs.Values {
			if within(pos, vs.Names[i]) || within(pos, vs.Values[i]) {
				return identName(vs.Names[i])
			}
		}
		return ""
	}
	for _, n := range vs.Names {
		if within(pos, n) {
			return identName(n)
		}
	}
	names := make([]ast.Expr, len(vs.Names))
	for i, n := range vs.Names {
		names[i] = n
	}
	return soleCloserName(pass, names)
}

func within(pos token.Pos, e ast.Expr) bool {
	return pos >= e.Pos() && pos <= e.End()
}

// soleCloserName returns the single closer-typed identifier among lhs, or "" when
// there is none or more than one.
func soleCloserName(pass *analysis.Pass, lhs []ast.Expr) string {
	name := ""
	for _, l := range lhs {
		id, ok := l.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}
		obj := pass.TypesInfo.Defs[id]
		if obj == nil {
			obj = pass.TypesInfo.Uses[id]
		}
		if obj != nil && implementsCloser(obj.Type()) {
			if name != "" {
				return "" // ambiguous: more than one closer
			}
			name = id.Name
		}
	}
	return name
}

func identName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
		return id.Name
	}
	return ""
}

// discardedCallNames maps the source positions of bare (unassigned) closer-returning
// calls to their callee name. Several candidate positions are recorded per call so
// the lookup matches whichever position the SSA instruction reports.
func discardedCallNames(pass *analysis.Pass) map[token.Pos]string {
	names := map[token.Pos]string{}
	record := func(call *ast.CallExpr, extra ...token.Pos) {
		if call == nil || !callReturnsCloser(pass.TypesInfo, call) {
			return
		}
		name := calleeName(call)
		names[call.Pos()] = name
		names[call.Lparen] = name
		for _, p := range extra {
			names[p] = name
		}
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(node ast.Node) bool {
			switch stmt := node.(type) {
			case *ast.ExprStmt:
				if call, ok := stmt.X.(*ast.CallExpr); ok {
					record(call)
				}
			case *ast.GoStmt:
				record(stmt.Call, stmt.Go)
			case *ast.DeferStmt:
				record(stmt.Call, stmt.Defer)
			}
			return true
		})
	}
	return names
}

func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

func callReturnsCloser(typesInfo *types.Info, call *ast.CallExpr) bool {
	switch t := typesInfo.Types[call].Type.(type) {
	case *types.Named, *types.Pointer:
		// Single return
		return types.Implements(t, closerType)
	case *types.Tuple:
		// Multiple returns, we check if any of them is an io.Closer
		for i := 0; i < t.Len(); i++ {
			if types.Implements(t.At(i).Type(), closerType) {
				return true
			}
		}
	}
	return false
}
