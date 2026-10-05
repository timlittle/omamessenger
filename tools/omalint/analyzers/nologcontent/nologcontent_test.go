package nologcontent

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestNoLogContent(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "github.com/timlittle/omamessenger/backend/internal/store")
}
