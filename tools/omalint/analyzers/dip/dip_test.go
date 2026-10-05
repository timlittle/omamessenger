package dip

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestDIP(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "github.com/timlittle/omamessenger/backend/internal/app")
}
