package mustclose

import (
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

var closerType *types.Interface

func init() {
	namedErrorType := types.Universe.Lookup("error").Type()

	sig := types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(types.NewVar(token.NoPos, nil, "", namedErrorType)), false)
	closeMethod := types.NewFunc(token.NoPos, nil, "Close", sig)
	closerType = types.NewInterfaceType([]*types.Func{closeMethod}, nil).Complete()
}

func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "mustclose",
		Doc:      "reports values that implement io.Closer but for which Close is never called",
		Run:      run,
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
	}
}

func run(pass *analysis.Pass) (interface{}, error) {
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	var diags []analysis.Diagnostic
	for _, fn := range ssaResult.SrcFuncs {
		diags = append(diags, analyzeFunc(fn)...)
	}

	sort.Slice(diags, func(i, j int) bool { return diags[i].Pos < diags[j].Pos })
	for _, d := range diags {
		pass.Report(d)
	}
	return nil, nil
}

// analyzeFunc reports closer values created in fn that are neither closed nor
// have their ownership transferred out of the function.
func analyzeFunc(fn *ssa.Function) []analysis.Diagnostic {
	if fn.Blocks == nil {
		return nil // external or generic function without a body
	}

	// An Alloc that receives a whole closer value (`*alloc = closer`) is just the
	// addressable spill slot for that value (e.g. a type-switch binding), not a
	// fresh origin. Tracking the stored value alone avoids double-reporting
	spill := map[ssa.Value]bool{}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if st, ok := instr.(*ssa.Store); ok {
				if alloc, ok := st.Addr.(*ssa.Alloc); ok && implementsCloser(st.Val.Type()) {
					spill[alloc] = true
				}
			}
		}
	}

	var diags []analysis.Diagnostic
	report := func(pos token.Pos) {
		diags = append(diags, analysis.Diagnostic{Pos: pos, Message: "Close is not called"})
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			// A closer bound to a value (assigned, allocated, extracted, asserted)
			if v := closerValueOrigin(instr); v != nil {
				if spill[v] {
					continue // storage for a value already tracked
				}
				if !isHandled(v) {
					report(originPos(v))
				}
				continue
			}
			// A closer result that is discarded and can never be closed
			// bare go/defer call, or a multi-value call whose returned closer is unused
			if pos, ok := discardedCloserResult(instr); ok {
				report(pos)
			}
		}
	}
	return diags
}

// originPos returns the source position for a closer value
func originPos(v ssa.Value) token.Pos {
	if v.Pos() != token.NoPos {
		return v.Pos()
	}
	// ssa.Extract doesn't carry a position of its own. So return the one from the tuple.
	if ext, ok := v.(*ssa.Extract); ok {
		return ext.Tuple.Pos()
	}
	return token.NoPos
}

// closerValueOrigin returns the ssa.Value produced by instr that represents a
// freshly created io.Closer the current function is responsible for, or nil.
func closerValueOrigin(instr ssa.Instruction) ssa.Value {
	switch it := instr.(type) {
	case *ssa.Alloc:
		// Alloc.Type() is always *T. In SSA, both `var x T` (zero-value) and
		// `x := &T{}` (explicit allocation) appear as Alloc instructions.
		// Without source-level information, we can't distinguish them perfectly.
		// We treat all Allocs as potential allocations, which means
		// `var x io.Closer; use(&x)` will also be flagged as a leak
		if implementsCloser(it.Type()) {
			return it
		}
	case *ssa.Call:
		// A single closer return value (whether assigned or used as a bare
		// statement: both look identical after SSA lifting).
		// Skip no-op closer factories like io.NopCloser.
		if implementsCloser(it.Type()) && !isNopCloser(it) {
			return it
		}
	case *ssa.Extract:
		// One component of a multi-value result (e.g. os.Create's *os.File).
		if implementsCloser(it.Type()) {
			return it
		}
	case *ssa.TypeAssert:
		// z := y.(SomeCloser)
		if !it.CommaOk && implementsCloser(it.AssertedType) {
			return it
		}
	}
	return nil
}

// discardedCloserResult reports the position of a call whose io.Closer result is
// discarded and therefore can never be closed. This covers go/defer statements
// (whose results are always dropped) and multi-value calls whose closer
// component is never extracted. No-op closer factories are skipped.
func discardedCloserResult(instr ssa.Instruction) (token.Pos, bool) {
	switch it := instr.(type) {
	case *ssa.Go:
		if signatureReturnsCloser(it.Common()) && !isNopCloser(it) {
			return it.Pos(), true
		}
	case *ssa.Defer:
		if signatureReturnsCloser(it.Common()) && !isNopCloser(it) {
			return it.Pos(), true
		}
	case *ssa.Call:
		// Bare call statement with single-value closer result (no referrers)
		if implementsCloser(it.Type()) && !isNopCloser(it) {
			refs := it.Referrers()
			if refs == nil || len(*refs) == 0 {
				return it.Pos(), true
			}
		}
		// Multi-value return. check for unextracted closer components
		tuple, ok := it.Type().(*types.Tuple)
		if !ok {
			return 0, false
		}
		for i := 0; i < tuple.Len(); i++ {
			if implementsCloser(tuple.At(i).Type()) && !hasExtract(it, i) {
				if !isNopCloser(it) {
					return it.Pos(), true
				}
			}
		}
	}
	return 0, false
}

func signatureReturnsCloser(common *ssa.CallCommon) bool {
	results := common.Signature().Results()
	for i := 0; i < results.Len(); i++ {
		if implementsCloser(results.At(i).Type()) {
			return true
		}
	}
	return false
}

// hasExtract reports whether the multi-value call result at index is extracted
// by an Extract instruction. multi-value returns become tuples in ssa, and
// callers that want to use a specific component emit an Extract instruction at
// that index. If a closer result is never extracted, it was never retrieved,
// so it can never be closed (and should be flagged as an error).
func hasExtract(call *ssa.Call, index int) bool {
	refs := call.Referrers()
	if refs == nil {
		return false
	}
	for _, instr := range *refs {
		if ext, ok := instr.(*ssa.Extract); ok && ext.Index == index {
			return true
		}
	}
	return false
}

// isNopCloser reports whether a call or call instruction is to a known no-op
// closer factory (e.g. io.NopCloser), whose Close method does nothing and thus
// doesn't need to be invoked.
func isNopCloser(cc interface{}) bool {
	var common *ssa.CallCommon
	switch c := cc.(type) {
	case *ssa.Call:
		common = c.Common()
	case *ssa.Go:
		common = c.Common()
	case *ssa.Defer:
		common = c.Common()
	default:
		return false
	}
	return isNopCloserCall(common)
}

// todo: change this into a user configurable allowlist?
func isNopCloserCall(common *ssa.CallCommon) bool {
	callee := common.StaticCallee()
	if callee == nil {
		return false
	}
	pkg := callee.Pkg
	if pkg == nil {
		return false
	}
	// Allowlist of functions that return no-op closers.
	// Format: "package/path.FuncName"
	fqn := pkg.Pkg.Path() + "." + callee.Name()
	switch fqn {
	case "io.NopCloser":
		return true
	}
	return false
}

// isHandled reports whether the closer value is closed or its ownership escapes
// the current function, by walking the value's referrers
func isHandled(root ssa.Value) bool {
	visited := map[ssa.Value]bool{}
	var walk func(v ssa.Value) bool

	// storeEscapes reports whether storing the tracked value into addr transfers
	// ownership away from the current function: either into a location that
	// outlives it (a global, parameter, receiver field, captured variable, or a
	// heap object obtained elsewhere), or into a local aggregate that is itself
	// closed, returned or captured. A store into a purely local aggregate that is
	// only handed to another function is NOT an escape, consistent with the
	// convention that a callee does not close what it receives.
	storeEscapes := func(addr ssa.Value) bool {
		base := addr
		for {
			switch a := base.(type) {
			case *ssa.FieldAddr:
				base = a.X
				continue
			case *ssa.IndexAddr:
				base = a.X
				continue
			}
			break
		}
		if _, ok := base.(*ssa.Alloc); ok {
			return walk(base) // local aggregate: handled only if it escapes/closes
		}
		return true // global / parameter / receiver / captured / heap object
	}

	walk = func(v ssa.Value) bool {
		if visited[v] {
			return false
		}
		visited[v] = true
		refs := v.Referrers()
		if refs == nil {
			return false
		}
		for _, instr := range *refs {
			switch it := instr.(type) {
			case ssa.CallInstruction:
				// Close invoked on v, or v captured by a deferred/go closure.
				if isCloseCall(it.Common(), v) {
					return true
				}
				// v is being invoked as a function value (e.g. a bound method value or
				// an anonymous closure that captures the closer). Conservatively treat
				// any invocation as closing the captured value.
				if !it.Common().IsInvoke() && it.Common().Value == v {
					return true
				}
			case *ssa.Return:
				return true // returned to the caller
			case *ssa.Store:
				if it.Val == v && storeEscapes(it.Addr) {
					return true // ownership moved to a persistent owner
				}
			case *ssa.MakeClosure:
				// A closure that captures the value. Follow the closure through
				// its referrers to see if it's invoked or stored/returned.
				if walk(it) {
					return true
				}
			case *ssa.TypeAssert:
				// A type assertion v.(T) extracts v as type T.
				// If T implements Closer, the extracted result is tracked separately,
				// and ownership of v transfers to that result (so v is handled).
				// If T does not implement Closer, the assertion doesn't extract a
				// closer, so v itself remains unhandled.
				if implementsCloser(it.AssertedType) {
					return true // ownership transferred to extracted closer
				}
			case *ssa.MakeInterface:
				if walk(it) {
					return true
				}
			case *ssa.ChangeType:
				if walk(it) {
					return true
				}
			case *ssa.ChangeInterface:
				if walk(it) {
					return true
				}
			case *ssa.Convert:
				if walk(it) {
					return true
				}
			case *ssa.UnOp:
				if walk(it) {
					return true // load of a value copy
				}
			case *ssa.Phi:
				if walk(it) {
					return true
				}
			case *ssa.FieldAddr:
				if walk(it) {
					return true // reached via a field of a tracked aggregate
				}
			case *ssa.IndexAddr:
				if walk(it) {
					return true // reached via an element of a tracked aggregate
				}
			case *ssa.Slice:
				if walk(it) {
					return true // aggregate re-sliced (e.g. before return)
				}
			}
		}
		return false
	}
	return walk(root)
}

// isCloseCall reports whether common is a call to Close with v as its receiver.
func isCloseCall(common *ssa.CallCommon, v ssa.Value) bool {
	if common.IsInvoke() {
		return common.Method.Name() == "Close" && common.Value == v
	}
	if fn, ok := common.Value.(*ssa.Function); ok {
		return fn.Signature.Recv() != nil && fn.Name() == "Close" && len(common.Args) > 0 && common.Args[0] == v
	}
	return false
}

func implementsCloser(t types.Type) bool {
	return types.Implements(t, closerType)
}
