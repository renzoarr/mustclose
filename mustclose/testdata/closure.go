package testdata

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
