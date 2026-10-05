package layering

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestLayering(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer,
		"github.com/timlittle/omamessenger/backend/internal/connector",
	)
}

const internal = "github.com/timlittle/omamessenger/backend/internal/"

// TestPackageRules runs the analyzer on single-file packages built in memory,
// which lets a case choose the package path and whether the file is a test.
func TestPackageRules(t *testing.T) {
	for _, test := range []struct {
		name, packagePath, filename, source, want string
	}{
		{"tools forbidden", "github.com/timlittle/omamessenger/tools/toolcase", "fixture.go", `package toolcase; import _ "` + internal + `domain"`, "may not import"},
		{"docscheck allowance", "github.com/timlittle/omamessenger/tools/docscheck", "fixture.go", `package docscheck; import _ "` + internal + `api"`, ""},
		{"external test package follows its package", internal + "connector_test", "x_test.go", `package connector_test; import _ "` + internal + `app"`, "connector may not import"},
		{"external test package gets test-only imports", internal + "connector_test", "x_test.go", `package connector_test; import _ "` + internal + `store"`, ""},
		{"external test package imports the package under test", internal + "connector_test", "x_test.go", `package connector_test; import _ "` + internal + `connector"`, ""},
		{"per-package test-only import", internal + "connector/demo_test", "x_test.go", `package demo_test; import _ "` + internal + `app"`, ""},
		{"per-package test-only import is not for production", internal + "connector/demo", "demo.go", `package demo; import _ "` + internal + `app"`, "may not import"},
		{"unknown internal package", internal + "mystery", "mystery.go", `package mystery`, "no C10 layering rule"},
		{"main may import anything", "github.com/timlittle/omamessenger/backend", "main.go", `package main; import _ "` + internal + `store"`, ""},
		{"generated test main ignored", internal + "connector.test", "testmain.go", `package main; import _ "` + internal + `app"`, ""},
		{"non-project package ignored", "example.com/other", "other.go", `package other; import _ "` + internal + `store"`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, test.filename, test.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: types.NewPackage(test.packagePath, "fixture"), Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
			if _, err := Analyzer.Run(pass); err != nil {
				t.Fatal(err)
			}
			if test.want == "" && len(diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", diagnostics)
			}
			if test.want != "" && (len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, test.want)) {
				t.Fatalf("expected %q, got %#v", test.want, diagnostics)
			}
		})
	}
}
