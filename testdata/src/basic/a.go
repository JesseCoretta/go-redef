package basic

func f() {
	x := 1
	_ = x

	if true {
		x := 2 // want "redefined"
		_ = x
	}
}
