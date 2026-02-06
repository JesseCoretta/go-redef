package guard

func g() error { return nil }
func h() error { return nil }

func f() {
	err := g()
	if err != nil {
		return
	}

	if err := h(); err != nil { // want "redefined"
		return
	}
}
