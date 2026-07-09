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
	return ptrCloser{}
}

func newPtrCloserPtr() *ptrCloser {
	return &ptrCloser{}
}

func newValCloser() valCloser {
	return valCloser{}
}

func newValCloserPtr() *valCloser {
	return &valCloser{}
}

func multipleImplementations() (valCloser, valCloser) {
	return valCloser{}, valCloser{}
}

func errorChecker(err error) {
	if err != nil {
		panic("fail")
	}
}

// someFunc creates a Closer object but doesn't call close nor returns it, so it should be reported by the analyzer
func someFunc() {
	e := valCloser{} // want "Close is not called on e"
	_ = e
}

// someFunc2 creates a Closer object and returns it instead of closing it
func someFunc2() valCloser {
	e := valCloser{} // ok. is returned
	return e
}

// someFunc3 creates a Closer object and returns it instead of closing it
func someFunc3() *ptrCloser {
	e := &ptrCloser{} // ok. is returned
	return e
}

// someFunc4 creates a Closer object and returns it instead of closing it
func someFunc4() (string, *ptrCloser) {
	e := &ptrCloser{} // ok. is returned
	return "test", e
}

type structWithCloser struct {
	field1 *ptrCloser
}

/* false-positives
func returnCloserInStruct() structWithCloser {
	a := &example{} // false positive: we are returning it in the struct so we shouldn't close it here
	b := structWithCloser{field1: a}
	return b
}

func returnCloserInStruct2() structWithCloser {
	a := &example{} // false positive: we are returning it in the struct so we shouldn't close it here
	return structWithCloser{field1: a}
}
*/

func zeroValues() {
	// these are currently false positives. They should be ignored

	var a ptrCloser  // this does not implement io.Closer. Method has pointer receiver
	var b *ptrCloser // want "Close is not called on b"
	var c valCloser  // want "Close is not called on c"
	var d *valCloser // want "Close is not called on d"

	aa := ptrCloser{} // this does not implement io.Closer. Method has pointer receiver
	cc := valCloser{} // want "Close is not called on cc"

	var i io.Closer      // want "Close is not called on i"
	var rc io.ReadCloser // want "Close is not called on rc"

	var m1, m2 *ptrCloser // want "Close is not called on m1" "Close is not called on m2"

	_, _, _, _, _, _, _, _, _, _ = a, b, c, d, aa, cc, i, rc, m1, m2
}

func main() {
	b := &ptrCloser{}    // want "Close is not called on b"
	c := valCloser{X: 1} // want "Close is not called on c"
	d := &valCloser{}    // want "Close is not called on d"
	_, _, _ = b, c, d

	var f valCloser = newValCloser()
	f.Close()

	var g valCloser = newValCloser()
	_ = g.Close()

	var h valCloser = newValCloser()
	defer h.Close()

	var i io.Closer = &ptrCloser{} // want "Close is not called on i"
	_ = i                          // to avoid unused variable error

	var k io.ReadCloser // want "Close is not called on k"
	_ = k               // to avoid unused variable error

	l, err := os.Create("/tmp/blah") // want "Close is not called on l"
	_, _ = l, err                    // to avoid unused variable error

	m := newValCloser() // want "Close is not called on m"
	_ = m

	n := newValCloserPtr()
	defer n.Close()

	o := newValCloserPtr()
	go o.Close()

	newValCloserPtr()       // want "Close is not called on the result of newValCloserPtr"
	go newValCloserPtr()    // want "Close is not called on the result of newValCloserPtr"
	defer newValCloserPtr() // want "Close is not called on the result of newValCloserPtr"

	os.Create("/tmp/blah")       // want "Close is not called on the result of Create"
	go os.Create("/tmp/blah")    // want "Close is not called on the result of Create"
	defer os.Create("/tmp/blah") // want "Close is not called on the result of Create"

	// declare p before using it as lhs with :=
	var p valCloser                   // want "Close is not called on p"
	p, q := multipleImplementations() // want "Close is not called on q"
	_, _ = p, q

	r, s := multipleImplementations() // want "Close is not called on r" "Close is not called on s"
	_, _ = r, s

	var t valCloser = newValCloser()
	defer func() { errorChecker(t.Close()) }()

	var u any = valCloser{}
	switch a := u.(type) {
	case valCloser:
		b := a // want "Close is not called on b"
		_ = b
	default:
	}

	var v any = valCloser{}
	switch a := v.(type) {
	case valCloser:
		fmt.Println(a) // limitation
	default:
	}

	w := structWithCloser{field1: &ptrCloser{}}
	w.field1.Close() // multiple SelectorExpr

	var x io.ReadCloser = &readClose{} // want "Close is not called on x"
	_ = x

	var y any = newValCloser()
	z := y.(valCloser) // want "Close is not called on z"
	_ = z

	// _ = newValCloserPtr() // "Close is not called on the result of newValCloserPtr()"

	// aa, _ := os.Open("file.txt")
	// bb := aa   // assuming aa is a pointer
	// bb.Close() // we shouldn't fail on aa not being closed
}
