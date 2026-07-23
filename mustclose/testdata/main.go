package testdata

import (
	"fmt"
	"io"
	"os"
)

type ptrCloser struct{ X int }

func (e *ptrCloser) Close() error {
	return nil
}

type valCloser struct{ X int }

func (e valCloser) Close() error {
	return nil
}

type readClose struct{ X int }

func (e *readClose) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (e *readClose) Close() error {
	return nil
}

func newptrCloserVal() ptrCloser {
	return ptrCloser{X: 1}
}

func newPtrCloserPtr() *ptrCloser {
	return &ptrCloser{X: 1}
}

func newValCloser() valCloser {
	return valCloser{X: 1}
}

func newValCloserPtr() *valCloser {
	return &valCloser{X: 1}
}

func multipleImplementations() (valCloser, valCloser) {
	return valCloser{X: 1}, valCloser{X: 2}
}

func errorChecker(err error) {
	if err != nil {
		panic("fail")
	}
}

// someFunc creates a Closer object but doesn't call close nor returns it, so it should be reported by the analyzer
func someFunc() {
	e := newValCloser() // want "Close is not called"
	_ = e
}

// someFunc2 creates a Closer object and returns it instead of closing it
func someFunc2() valCloser {
	e := newValCloser() // ok. is returned
	return e
}

// someFunc3 creates a Closer object and returns it instead of closing it
func someFunc3() *ptrCloser {
	e := newPtrCloserPtr() // ok. is returned
	return e
}

// someFunc4 creates a Closer object and returns it instead of closing it
func someFunc4() (string, *ptrCloser) {
	e := newPtrCloserPtr() // ok. is returned
	return "test", e
}

type structWithCloser struct {
	field1 *ptrCloser
}

// returnCloserInStruct stores a closer in a struct that is then returned, so the
// closer's ownership escapes and it must not be flagged (was limitation #5).
func returnCloserInStruct() structWithCloser {
	a := &ptrCloser{} // ok: escapes via the returned struct
	b := structWithCloser{field1: a}
	return b
}

func returnCloserInStruct2() structWithCloser {
	a := &ptrCloser{} // ok: escapes via the returned struct literal
	return structWithCloser{field1: a}
}

// commaOkAssert exercises comma-ok type assertions: the extracted closer is
// tracked like any other origin.
func commaOkAssert(x any) {
	if c, ok := x.(io.Closer); ok {
		c.Close() // ok: closed
	}
	if c, ok := x.(valCloser); ok { // want "Close is not called"
		_ = c
	}
}

// A closer stored into a local aggregate that is discarded never escapes and is
// never closed, so it is reported.
func discardedInSlice() {
	a := &ptrCloser{} // want "Close is not called"
	s := []io.Closer{a}
	_ = s
}

func discardedInStruct() {
	a := &ptrCloser{} // want "Close is not called"
	b := structWithCloser{field1: a}
	_ = b
}

// A closer stored into an aggregate that is returned escapes to the caller.
func returnedInSlice() []io.Closer {
	a := &ptrCloser{} // ok: escapes via the returned slice
	return []io.Closer{a}
}

func zeroValues() {
	// these are currently false positives. They should be ignored

	var a ptrCloser  // this does not implement io.Closer. Method has pointer receiver
	var b *ptrCloser // ignored
	var c valCloser  // ignored
	var d *valCloser // ignored

	aa := ptrCloser{} // this does not implement io.Closer. Method has pointer receiver
	cc := valCloser{} // ignored

	var i io.Closer      // ignored
	var rc io.ReadCloser // ignored

	var m1, m2 *ptrCloser // ignored

	_, _, _, _, _, _, _, _, _, _ = a, b, c, d, aa, cc, i, rc, m1, m2
}

// nopCloser exercises io.NopCloser, which should be skipped (allowed).
func nopCloser(r io.Reader) {
	// io.NopCloser returns a noop closer; Close calls on it do nothing,
	// so flagging its use as an error would be incorrect.
	nr := io.NopCloser(r) // ok: no-op closer, no warning
	_ = nr

	// other use patterns.
	io.NopCloser(r)       // ok: no-op closer
	defer io.NopCloser(r) // ok: no-op closer
	go io.NopCloser(r)    // ok: no-op closer
}

func methodValueNotInvoked() {
	c := newPtrCloserPtr() // want "Close is not called"
	closeFn := c.Close
	_ = closeFn // closeFn never invoked
}

func methodValueInvoked() {
	c := newPtrCloserPtr()
	closeFn := c.Close
	closeFn()
}

func typeAssertToNonCloser() {
	c := newValCloser() // want "Close is not called"
	var x any = c
	_, _ = x.(fmt.Stringer) // assert to non-closer
	// c is still not closed
}

// Close is a package-level function — NOT the io.Closer.Close method.
// Calling it must not suppress a "Close is not called" diagnostic.
func Close(*ptrCloser) {}

func packageFunctionNamedClose() {
	val := newPtrCloserPtr() // want "Close is not called"
	Close(val)               // free function, not the io.Closer.Close method
}

func addressTakenZeroValue() {
	// warning is raised because we take the address of it below
	var c ptrCloser // want "Close is not called"
	discardZeroCloser(&c)
}

func receivedCloserNotClosed(c io.Closer) {
	_ = c
}

// Helper: consumes address of closer
func discardZeroCloser(*ptrCloser) {}

func main() {
	b := &ptrCloser{}    // want "Close is not called"
	c := valCloser{X: 1} // want "Close is not called"
	d := &valCloser{}    // want "Close is not called"
	_, _, _ = b, c, d

	var f valCloser = newValCloser()
	f.Close()

	var g valCloser = newValCloser()
	_ = g.Close()

	var h valCloser = newValCloser()
	defer h.Close()

	var i io.Closer = &ptrCloser{} // want "Close is not called"
	_ = i                          // to avoid unused variable error

	var k io.ReadCloser // ignored: nil zero value, nothing to close
	_ = k               // to avoid unused variable error

	l, err := os.Create("/tmp/blah") // want "Close is not called"
	_, _ = l, err                    // to avoid unused variable error

	m := newValCloser() // want "Close is not called"
	_ = m

	n := newValCloserPtr()
	defer n.Close()

	o := newValCloserPtr()
	go o.Close()

	newValCloserPtr()       // want "Close is not called"
	go newValCloserPtr()    // want "Close is not called"
	defer newValCloserPtr() // want "Close is not called"

	os.Create("/tmp/blah")       // want "Close is not called"
	go os.Create("/tmp/blah")    // want "Close is not called"
	defer os.Create("/tmp/blah") // want "Close is not called"

	// declare p before using it as lhs assignment with :=
	var p valCloser
	p, q := multipleImplementations() // want "Close is not called" "Close is not called"
	_, _ = p, q

	r, s := multipleImplementations() // want "Close is not called" "Close is not called"
	_, _ = r, s

	var t valCloser = newValCloser()
	defer func() { errorChecker(t.Close()) }()

	var u any = newValCloser()
	switch a := u.(type) {
	case valCloser: // want "Close is not called"
		b := a
		_ = b
	default:
	}
	// A closer bound in a type switch is now tracked: if the binding is neither
	// closed nor allowed to escape, it is reported (was limitation #4).
	var v any = newValCloser()
	switch a := v.(type) {
	case valCloser: // want "Close is not called"
		_ = a.X
	default:
	}

	// A closer passed into a variadic ...any slice cannot be traced to a Close,
	// so — following the convention that a callee does not close what it
	// receives — it is assumed open and reported.
	var v2 any = newValCloser()
	switch a := v2.(type) {
	case valCloser: // want "Close is not called"
		fmt.Println(a)
	default:
	}

	w := structWithCloser{field1: &ptrCloser{}}
	w.field1.Close() // multiple SelectorExpr

	var x io.ReadCloser = &readClose{} // want "Close is not called"
	_ = x

	var y any = newValCloser()
	z := y.(valCloser) // want "Close is not called"
	_ = z

	// Closing through a method value clears the warning (was limitation #6).
	ba := newValCloserPtr() // ok: closed via a method value
	closeFn := ba.Close
	closeFn()

	// Closing through an alias clears the warning (was limitation #2/aliasing).
	ca, _ := os.Create("/tmp/alias")
	cb := ca
	cb.Close()
}

func wrap(f func() error) error {
	return f()
}

func usesIt(origns ptrCloser) error { return nil }

func hello(origns ptrCloser) error {
	// even though origns doesn't implement io.Closer, because it's used in a closure, the compiler stores it in an addressable slot (ssa.Alloc), and it get's picked up
	return wrap(func() error {
		return usesIt(origns)
	})
}

func hello2(origns ptrCloser) error {
	return usesIt(origns)
}

/*
// tCleanup simulates testing.T.Cleanup function, which takes a func() and calls it at the end of the test.
// This is a common pattern for closing resources in tests.
func tCleanup(f func()) {
	f()
}

// current implementation doesn't see `a` as closed
func myTestFunc() {
	a := newValCloser()
	tCleanup(func() {
		a.Close()
	})
}
*/
