package redef

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestRedef(t *testing.T) {
	testdata := analysistest.TestData()

	// strict mode
	analysistest.Run(t, testdata, Analyzer,
		"sample", "sample2", "basic", "shortif", "deadouter",
		"errshadow", "guard", "table",
		"latertrue", "laterfalse",
		"tablematch", "tablenomatch",
		"guardonly", "guardnot",
	)

	// allow-dead-outer → suppression expected
	Analyzer.Flags.Set("allow-dead-outer", "true")
	analysistest.Run(t, testdata, Analyzer, "latertrue")
	Analyzer.Flags.Set("allow-dead-outer", "false")

	// allow-table-tests → suppression expected
	Analyzer.Flags.Set("allow-table-tests", "true")
	analysistest.Run(t, testdata, Analyzer, "tablematch")
	Analyzer.Flags.Set("allow-table-tests", "false")

	// allow-guard-shadow → suppression expected
	Analyzer.Flags.Set("allow-guard-shadow", "true")
	analysistest.Run(t, testdata, Analyzer, "guardonly")
	Analyzer.Flags.Set("allow-guard-shadow", "false")

}
