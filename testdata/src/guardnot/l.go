package guardnot

func g() error { return nil }

func f() {
	err := g()
	if err != nil {
		return
	}

	if err := g(); err != nil { // want "redefined"
		panic(err) // not a guard clause
	}
}
