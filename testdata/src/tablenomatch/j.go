package tablenomatch

func f() {
	x := 1
	_ = x

	tests := []int{1, 2, 3}
	for _, tt := range tests {
		x := tt // want "redefined"
		_ = x
	}
}
