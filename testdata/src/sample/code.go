package sample

func g() (int, error) { return 0, nil }
func h() error        { return nil }

func f() {
	_, err := g()
	if err != nil {
		panic(err)
	}

	if err := h(); err != nil { // want "redefined"
		panic(err)
	}
}
