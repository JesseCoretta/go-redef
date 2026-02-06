package main

import (
	"github.com/JesseCoretta/go-redef"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(redef.Analyzer)
}
