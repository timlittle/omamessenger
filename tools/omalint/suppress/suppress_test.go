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

// A suppression must only cover the next line of its own file. Positions in
// a FileSet are global, so a comment in an earlier-parsed file has a smaller
// Pos and, compared by line number alone, appeared to sit "above" every line
// of later files.
func TestCheckDoesNotSuppressAcrossFiles(t *testing.T) {
	fset := token.NewFileSet()
	first, err := parser.ParseFile(fset, "a.go", `package fixture

func one() {}
func two() {}
func three() {}

//omalint:ignore size documented reason
func suppressed() {}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	second, err := parser.ParseFile(fset, "b.go", `package fixture

func violates() {}
`, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{first, second}, Report: func(analysis.Diagnostic) {}}
	if !Check(pass, "size", first.Decls[3].Pos()) {
		t.Fatal("suppression must cover the next line in its own file")
	}
	if Check(pass, "size", second.Decls[0].Pos()) {
		t.Fatal("suppression in a.go must not cover a finding in b.go")
	}
}
