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

// someFunc creates a Closer object but doesn't call close nor returns it, so it should be reported by the analyzer
func someFunc() {
	e := example2{} // want "Closed is not called on e"
	_ = e
}

// someFunc2 creates a Closer object but doesn't call close but does return it
func someFunc2() example2 {
	e := example2{} // ok. is returned
	return e
}

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

	// TODO: this should report some error since the method does return an implementation of io.Closer, but because I don't assign it to a variable we can't close it
	// Or, we should treat assignment to _ as an ignore? similar to errorcheck?
	// _ = getImpl2Ptr() // "Closed is not called on the result of getImpl2Ptr()"

	// TODO: this should report some error since the method does return an implementation of io.Closer, but because I don't assign it to a variable we can't close it
	// Or, we should treat assignment to _ as an ignore? similar to errorcheck?
	// getImpl2Ptr() // "Closed is not called on the result of getImpl2Ptr()"

	/* TODO
	   var a = os.Open("file.txt")
	   // assuming a is a pointer
	   b := a
	   b.Close()
	   // we shouldn't fail on a not being closed
	*/
}
