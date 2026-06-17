package mustclose

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/analysis"
	// "golang.org/x/tools/go/packages"
)

var closerType *types.Interface
var errorType *types.Interface

func init() {
	namedErrorType := types.Universe.Lookup("error").Type()
	errorType = namedErrorType.Underlying().(*types.Interface)

	sig := types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(types.NewVar(token.NoPos, nil, "", namedErrorType)), false)
	closeMethod := types.NewFunc(token.NoPos, nil, "Close", sig)
	closerType = types.NewInterfaceType([]*types.Func{closeMethod}, nil).Complete()
}

func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "mustclose",
		Doc:      "reports values that implement io.Closer but for which Close is never called",
		Run:      run,
		Requires: []*analysis.Analyzer{}, // I should parse all types definitions here
	}
}

func run(pass *analysis.Pass) (interface{}, error) {
	// closers that aren't closed
	open := map[types.Object]struct{}{}
	// returned closers that are not assigned
	unassigned := map[types.Object]struct{}{}

	inspect := func(node ast.Node) bool {
		// find all variables that implement io.Closer
		switch stmt := node.(type) {
		case *ast.ValueSpec:
			// variable declaration
			var found bool
			for _, name := range stmt.Names {
				obj := pass.TypesInfo.Defs[name]
				if obj == nil {
					continue
				}
				if types.Implements(obj.Type(), closerType) {
					open[obj] = struct{}{}
					found = true
				}
			}
			if found {
				return false
			}
		case *ast.AssignStmt:
			if stmt.Tok != token.DEFINE {
				break
			}
			// short variable declaration
			var found bool
			for _, lhs := range stmt.Lhs {
				// could still be a var assignment iso definition when you have multiple lhs vars with := , if at least one of them is a declaration. In that case, ok will be false
				obj, ok := pass.TypesInfo.Defs[lhs.(*ast.Ident)]
				if !ok || obj == nil { // ok is true and obj is nil on the use of := in a switch
					continue
				}
				if types.Implements(obj.Type(), closerType) {
					open[obj] = struct{}{}
					found = true
				}
			}
			if found {
				return false
			}
		default:
		}

		// find all calls that return an io.closer (and not assigned to a variable)
		switch stmt := node.(type) {
		case *ast.ExprStmt:
			// find unassigned function calls
			if call, ok := stmt.X.(*ast.CallExpr); ok {
				if callReturnsCloser(pass.TypesInfo, call) {
					name := ""
					if id, ok := call.Fun.(*ast.Ident); ok {
						name = id.Name
					} else if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
						name = selector.Sel.Name
					}
					obj := types.NewFunc(stmt.Pos(), nil, name, nil)
					unassigned[obj] = struct{}{}
					return false
				}
			}
		case *ast.GoStmt:
			// find go calls that return io.Closers
			if callReturnsCloser(pass.TypesInfo, stmt.Call) {
				name := ""
				if id, ok := stmt.Call.Fun.(*ast.Ident); ok {
					name = id.Name
				} else if selector, ok := stmt.Call.Fun.(*ast.SelectorExpr); ok {
					name = selector.Sel.Name
				}
				obj := types.NewFunc(stmt.Call.Pos(), nil, name, nil)
				unassigned[obj] = struct{}{}
				return false
			}
		case *ast.DeferStmt:
			// find defer calls that return io.Closers
			if callReturnsCloser(pass.TypesInfo, stmt.Call) {
				name := ""
				if id, ok := stmt.Call.Fun.(*ast.Ident); ok {
					name = id.Name
				} else if selector, ok := stmt.Call.Fun.(*ast.SelectorExpr); ok {
					name = selector.Sel.Name
				}
				obj := types.NewFunc(stmt.Call.Pos(), nil, name, nil)
				unassigned[obj] = struct{}{}
				return false
			}
		default:
		}

		// find all Close() calls, and io.Closer returns
		switch stmt := node.(type) {
		case *ast.CallExpr:
			selector, ok := stmt.Fun.(*ast.SelectorExpr)
			// Closer is a method, so it should always be a selector expression, if it's not we can ignore it
			if !ok {
				break
			}
			if selector.Sel.Name == "Close" && stmt.Args == nil && stmt.Ellipsis == token.NoPos && callReturnsSingleError(pass.TypesInfo, stmt) {
				// Close called so we remove the X from the map
				id, ok := selector.X.(*ast.Ident)
				if !ok {
					// one case in which we hit this is in nested SelectorExprs, e.g: `resp.Body.Close()`
					break
				}
				obj := pass.TypesInfo.Uses[id]
				delete(open, obj)
			}
		case *ast.ReturnStmt:
			for _, result := range stmt.Results {
				ident, ok := result.(*ast.Ident)
				if !ok {
					continue
				}
				obj := pass.TypesInfo.Uses[ident]
				delete(open, obj)
			}

		default:
		}
		return true
	}

	for _, f := range pass.Files {
		ast.Inspect(f, inspect)
	}

	diags := make([]analysis.Diagnostic, 0, len(open)+len(unassigned))
	for id := range open {
		diags = append(diags, analysis.Diagnostic{
			Pos:     id.Pos(),
			Message: fmt.Sprintf("Close is not called on %s", id.Name()),
		})
	}
	for id := range unassigned {
		diags = append(diags, analysis.Diagnostic{
			Pos:     id.Pos(),
			Message: fmt.Sprintf("Close is not called on the result of %s", id.Name()),
		})
	}

	sort.Slice(diags, func(i, j int) bool { return diags[i].Pos < diags[j].Pos })
	for _, d := range diags {
		pass.Report(d)
	}
	return nil, nil
}

func callReturnsSingleError(typesInfo *types.Info, call *ast.CallExpr) bool {
	t, ok := typesInfo.Types[call].Type.(*types.Named)
	if !ok {
		return false
	}
	return types.Implements(t, errorType)
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
