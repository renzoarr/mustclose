package mustclose

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	// "golang.org/x/tools/go/packages"
)

var closerType *types.Interface

func init() {
	ioPackage, err := importer.Default().Import("io")
	if err != nil {
		panic(fmt.Sprintf("failed to import io package: %s", err))
	}
	closerType = ioPackage.Scope().Lookup("Closer").Type().Underlying().(*types.Interface)
}

func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "mustclose",
		Doc:      "Checks that any var* implementing the io.Closer interface is closed.",
		Run:      run,
		Requires: []*analysis.Analyzer{}, // I should parse all types definitions here
	}
}

// question: what I have I have something like this
/*
var a = os.Open("file.txt")
// assuming a is a pointer
b := a
b.Close()
// I guess is shouldn't fail on var a ?
*/

func run(pass *analysis.Pass) (interface{}, error) {

	// closers that aren't closed
	open := map[types.Object]struct{}{}
	inspect := func(node ast.Node) bool {
		// TODO: we should check function calls that return an io.Closer as well, but aren't assigned to a variable
		// TODO: should we also check fields in structs? lilke http resp.Body which you need to call close on as well?

		// we will have this switch find the var declarations of variables that implement io.Closer
		// and also of function calls that return an io.Closer (implementation) and are not assigned to a variable
		switch stmt := node.(type) {
		case *ast.ValueSpec:
			fmt.Println("##### variable declartion found ######")
			fmt.Println("found var declaration: ", stmt.Names[0].Name)
			fmt.Println("declartion position: ", stmt.Pos()) // this is the same as the one below
			obj := pass.TypesInfo.Defs[stmt.Names[0]]
			fmt.Println("defines: ", obj.Pos()) // this is the same as the one above
			if types.Implements(obj.Type(), closerType) {
				fmt.Println("the var implements io.Closer")
				open[obj] = struct{}{}
			}
		case *ast.AssignStmt:
			if stmt.Tok != token.DEFINE {
				break
			}
			fmt.Println("##### short variable declaration found ######")
			for _, lhs := range stmt.Lhs {
				fmt.Println("found var declaration: ", lhs)
				fmt.Println("declaration position: ", lhs.Pos()) // this is the current position
				obj := pass.TypesInfo.Defs[lhs.(*ast.Ident)]
				fmt.Println("defines: ", obj.Pos()) // this is the same as the pair in the ValueSpec
				if types.Implements(obj.Type(), closerType) {
					fmt.Println("the var implements io.Closer")
					open[obj] = struct{}{}
				}
			}

		default:
		}

		// this switch should find all of the calls to Close() err and remove the vars from the map
		switch stmt := node.(type) {
		/*
			case *ast.AssignStmt:
				if stmt.Tok == token.DEFINE {
					break
				}
				fmt.Println("##### variable assignment found ######")
				for _, rhs := range stmt.Rhs {
					fmt.Println("found use of var: ", rhs)
					fmt.Println("use position: ", rhs.Pos()) // this is the current position
					// fixme: this fails on `_ = a.Close()`, since the right hand side is not an expresssion; This whole block is just and example
					fmt.Println("uses: ", pass.TypesInfo.Uses[rhs.(*ast.Ident)].Pos()) // this is the same as the pair in the ValueSpec
					// TODO: don't check uses, check if Close is called
					// delete(used, pass.TypesInfo.Uses[rhs.(*ast.Ident)])
				}
		*/
		case *ast.CallExpr:
			selector, ok := stmt.Fun.(*ast.SelectorExpr)
			if !ok { // Closer is a method, so it should always be a selector expression, if it's not we can ignore it
				// TODO: what if the close method is assigned to a variable and then called? like `closeFunc := a.Close; closeFunc()`
				break
			}
			fmt.Println("##### method call found ######")
			fmt.Println("found method call: ", stmt.Fun)
			fmt.Println("method call position: ", stmt.Pos())
			fmt.Println("method call receiver: ", selector.X)
			fmt.Println("method call method: ", selector.Sel.Name)
			if selector.Sel.Name == "Close" && stmt.Args == nil && stmt.Ellipsis == token.NoPos {
				// TODO: the return type should also be of type error, but I don't know how to check that
				// close called so we remove the X from the map
				obj := pass.TypesInfo.Uses[selector.X.(*ast.Ident)]
				fmt.Println("Close is called on: ", obj.Name())
				delete(open, obj)
			}
		default:
		}
		return true
	}

	for _, f := range pass.Files {
		ast.Inspect(f, inspect)
	}

	for id := range open {
		pass.Report(analysis.Diagnostic{
			Pos:     id.Pos(),
			Message: fmt.Sprintf("Closed is not called on %s", id.Name()),
		})
	}

	fmt.Println(open)
	return nil, nil
}

func callsClose(exp *ast.Expr) bool {
	return false
}
