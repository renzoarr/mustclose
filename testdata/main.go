package testdata

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

func main() {
	// this does not implement io.Closer. Method has pointer receiver
	// var aa io.Closer = example{} // this is invalid
	var a example
	_ = a
	// a.Close() // but I can call this?

	var b *example // want "Closed is not called on b"
	_ = b

	// directly implements closer
	var c example2 // want "Closed is not called on c"
	_ = c

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

	/*
		var f io.Closer = &example{}
		_ = f                        // to avoid unused variable error

		var g io.Closer
		_ = g           // to avoid unused variable error

		var h io.ReadCloser
		_ = h // to avoid unused variable error

		f, err := os.Create("/tmp/blah")
	*/
	// f, err := os.Create("/tmp/blah")
	// fmt.Println(err)
	// f, err = os.Create("/tmp/blah")
	// _ = f

	// TODO: some other type (not interface) that timplements io.closer from a package. So that it's a selector expresssion
}
