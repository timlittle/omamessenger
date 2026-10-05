package main

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
)

func TestAnalyzerRegistry(t *testing.T) {
	got := analyzers()
	if len(got) != 2 || got[0].Name != "layering" || got[1].Name != "size" {
		t.Fatalf("unexpected analyzer registry: %#v", got)
	}
}

func TestMainRunsRegisteredAnalyzers(t *testing.T) {
	called := false
	runMultichecker = func(got ...*analysis.Analyzer) { called = len(got) == 2 }
	defer func() { runMultichecker = multichecker.Main }()
	main()
	if !called {
		t.Fatal("main did not pass the analyzer registry to multichecker")
	}
}
