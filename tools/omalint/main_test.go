package main

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
)

func TestAnalyzerRegistry(t *testing.T) {
	got := analyzers()
	want := []string{"layering", "size", "complexity", "dip", "frozeniface", "nologcontent"}
	if len(got) != len(want) {
		t.Fatalf("unexpected analyzer registry: %#v", got)
	}
	for i, analyzer := range got {
		if analyzer.Name != want[i] {
			t.Fatalf("analyzer %d = %q, want %q", i, analyzer.Name, want[i])
		}
	}
}

func TestMainRunsRegisteredAnalyzers(t *testing.T) {
	called := false
	runMultichecker = func(got ...*analysis.Analyzer) { called = len(got) == 6 }
	defer func() { runMultichecker = multichecker.Main }()
	main()
	if !called {
		t.Fatal("main did not pass the analyzer registry to multichecker")
	}
}
