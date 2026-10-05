package size

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestSize(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "sizecase")
}
