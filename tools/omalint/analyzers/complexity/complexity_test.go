package complexity

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestComplexity(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "complexitycase")
}
