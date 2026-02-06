package redef

import (
    "testing"

    "golang.org/x/tools/go/analysis/analysistest"
)

func TestRedef(t *testing.T) {
    testdata := analysistest.TestData()
    analysistest.Run(t, testdata, Analyzer, "sample", "sample2")
}

