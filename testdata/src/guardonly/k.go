package guardonly

func g() error { return nil }

func f() {
	err := g()
	if err != nil {
		return
	}

	if err := g(); err != nil { // want "redefined"
		return
	}
}
