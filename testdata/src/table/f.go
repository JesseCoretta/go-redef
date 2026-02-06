package table

func f() {
	tests := []int{1, 2, 3}
	for _, tt := range tests {
		tt := tt // want "redefined"
		_ = tt
	}
}
