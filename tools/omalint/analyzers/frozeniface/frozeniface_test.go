package frozeniface

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestFrozenInterfaces(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "frozen/good", "frozen/added", "frozen/removed", "frozen/noninterface", "frozen/other")
}
