package latertrue

func f() {
	x := 1
	if true {
		x := 2 // want "redefined"
		_ = x
	}
	_ = x // outer used later
}
