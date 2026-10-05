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

func TestToolsLayering(t *testing.T) {
	for _, test := range []struct {
		name, packagePath, source, want string
	}{
		{"forbidden", "github.com/timlittle/omamessenger/tools/toolcase", `package toolcase; import _ "github.com/timlittle/omamessenger/backend/internal/domain"`, "may not import"},
		{"docscheck allowance", "github.com/timlittle/omamessenger/tools/docscheck", `package docscheck; import _ "github.com/timlittle/omamessenger/backend/internal/api"`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture.go", test.source, 0)
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
