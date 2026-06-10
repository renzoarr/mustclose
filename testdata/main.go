package testdata

import (
	"io"
	"os"
)

type example struct{}

func (e *example) Close() error {
	return nil
}

type example2 struct{}

func (e example2) Close() error {
	return nil
}

func getImpl() example {
	return example{}
}

func getImplPtr() *example {
	return &example{}
}

func getImpl2() example2 {
	return example2{}
}

func getImpl2Ptr() *example2 {
	return &example2{}
}

func multipleImplementations() (example2, example2) {
	return example2{}, example2{}
}

func errorChecker(err error) {
	if err != nil {
		panic("fail")
	}
}

// someFunc creates a Closer object but doesn't call close nor returns it, so it should be reported by the analyzer
func someFunc() {
	e := example2{} // want "Closed is not called on e"
	_ = e
}

// someFunc2 creates a Closer object and returns it instead of closing it
func someFunc2() example2 {
	e := example2{} // ok. is returned
	return e
}

type structWithCloser struct {
	field1 *example
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

func main() {
	// this does not implement io.Closer. Method has pointer receiver
	var a example
	_ = a

	// TBD: change this to not throw an error since it's nil
	var b *example // want "Closed is not called on b"
	_ = b

	// directly implements closer
	var c example2 // want "Closed is not called on c"
	_ = c

	// TBD: change this to not throw an error since it's nil
	// pointer to struct implements closer
	var d *example2 // want "Closed is not called on d"
	_ = d

	e := &example{} // want "Closed is not called on e"
	_ = e

	var f example2
	f.Close()

	var g example2
	_ = g.Close()

	var h example2
	defer h.Close()

	var i io.Closer = &example{} // want "Closed is not called on i"
	_ = i                        // to avoid unused variable error

	var j io.Closer // want "Closed is not called on j"
	_ = j           // to avoid unused variable error

	var k io.ReadCloser // want "Closed is not called on k"
	_ = k               // to avoid unused variable error

	l, err := os.Create("/tmp/blah") // want "Closed is not called on l"
	_, _ = l, err                    // to avoid unused variable error

	m := getImpl2() // want "Closed is not called on m"
	_ = m

	n := getImpl2Ptr()
	defer n.Close()

	o := getImpl2Ptr()
	go o.Close()

	getImpl2Ptr()       // want "Closed is not called on the result of getImpl2Ptr"
	go getImpl2Ptr()    // want "Closed is not called on the result of getImpl2Ptr"
	defer getImpl2Ptr() // want "Closed is not called on the result of getImpl2Ptr"

	os.Create("/tmp/blah")       // want "Closed is not called on the result of Create"
	go os.Create("/tmp/blah")    // want "Closed is not called on the result of Create"
	defer os.Create("/tmp/blah") // want "Closed is not called on the result of Create"

	// declare p before using it as lhs with :=
	var p example2                    // want "Closed is not called on p"
	p, q := multipleImplementations() // want "Closed is not called on q"
	_, _ = p, q

	r, s := multipleImplementations() // want "Closed is not called on r" "Closed is not called on s"
	_, _ = r, s

	var t example2
	defer func() { errorChecker(t.Close()) }()

	// TODO: causes panic:
	// var u any = "asdf"
	// switch a := u.(type) {
	// case string:
	// 	_ = a
	// default:
	// }

	// FIXME: causes panic
	// v := structWithCloser{field1: &example{}}
	// v.field1.Close() // multiple SelectorExpr

	// TODO: this should report some error since the method does return an implementation of io.Closer, but because I don't assign it to a variable we can't close it
	// Or, we should treat assignment to _ as an ignore? similar to errorcheck?
	// _ = getImpl2Ptr() // "Closed is not called on the result of getImpl2Ptr()"

	// TODO
	// aa, _ := os.Open("file.txt")
	// bb := aa   // assuming aa is a pointer
	// bb.Close() // we shouldn't fail on aa not being closed
}
