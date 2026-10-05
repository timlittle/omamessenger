// Command omalint runs the OmaMessenger Go architecture analyzers.
package main

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/timlittle/omamessenger/tools/omalint/analyzers/complexity"
	"github.com/timlittle/omamessenger/tools/omalint/analyzers/dip"
	"github.com/timlittle/omamessenger/tools/omalint/analyzers/frozeniface"
	"github.com/timlittle/omamessenger/tools/omalint/analyzers/layering"
	"github.com/timlittle/omamessenger/tools/omalint/analyzers/nologcontent"
	"github.com/timlittle/omamessenger/tools/omalint/analyzers/size"
)

func main() {
	runMultichecker(analyzers()...)
}

var runMultichecker = multichecker.Main

func analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{layering.Analyzer, size.Analyzer, complexity.Analyzer, dip.Analyzer, frozeniface.Analyzer, nologcontent.Analyzer}
}
