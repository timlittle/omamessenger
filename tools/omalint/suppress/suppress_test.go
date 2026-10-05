package suppress

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestCheckAndValidate(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", `package fixture
//omalint:ignore size documented reason
func allowed() {}
//omalint:ignore layering
func malformed() {}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
	if !Check(pass, "size", file.Decls[0].Pos()) {
		t.Fatal("expected documented suppression")
	}
	if Check(pass, "layering", file.Decls[1].Pos()) {
		t.Fatal("missing reason must not suppress")
	}
	Validate(pass)
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "suppression requires") {
		t.Fatalf("expected one missing-reason finding, got %#v", diagnostics)
	}
}

func TestCheckDoesNotSuppressWrongRuleOrDistantPosition(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", `package fixture
//omalint:ignore size documented reason

func target() {}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Report: func(analysis.Diagnostic) {}}
	if Check(pass, "layering", file.Decls[0].Pos()) || Check(pass, "size", file.Decls[0].Pos()) {
		t.Fatal("unexpected suppression across a blank line or for another rule")
	}
}
