package mustclose

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/ctrlflow"
	"golang.org/x/tools/go/ssa"
)

// buildSSADebug builds SSA for the package under analysis with GlobalDebug so
// *ssa.DebugRef instructions are emitted. Returns source functions (including
// closures), mirroring buildssa.SrcFuncs.
func buildSSADebug(pass *analysis.Pass) []*ssa.Function {
	cfgs := pass.ResultOf[ctrlflow.Analyzer].(*ctrlflow.CFGs)
	prog := ssa.NewProgram(pass.Fset, ssa.GlobalDebug)
	prog.SetNoReturn(cfgs.NoReturn)
	for _, p := range pass.Pkg.Imports() {
		prog.CreatePackage(p, nil, nil, true)
	}
	ssaPkg := prog.CreatePackage(pass.Pkg, pass.Files, pass.TypesInfo, false)
	ssaPkg.Build()

	var funcs []*ssa.Function
	var add func(*ssa.Function)
	add = func(f *ssa.Function) {
		funcs = append(funcs, f)
		for _, anon := range f.AnonFuncs {
			add(anon)
		}
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn, ok := pass.TypesInfo.Defs[fd.Name].(*types.Func); ok && fn != nil {
				if f := prog.FuncValue(fn); f != nil {
					add(f)
				}
			}
		}
	}
	return funcs
}

func sourceNames(fn *ssa.Function) map[ssa.Value]string {
	names := map[ssa.Value]string{}
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			if dr, ok := instr.(*ssa.DebugRef); ok && dr.Object() != nil {
				names[dr.X] = dr.Object().Name()
			}
		}
	}
	return names
}
