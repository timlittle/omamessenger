// Package layering checks internal package import edges.
package layering

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/timlittle/omamessenger/tools/omalint/rules"
	"github.com/timlittle/omamessenger/tools/omalint/suppress"
)

var Analyzer = &analysis.Analyzer{
	Name: "layering",
	Doc:  "check internal imports against the C10 layering table",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	suppress.Validate(pass)
	if strings.HasPrefix(pass.Pkg.Path(), rules.ToolsPrefix) {
		return runTools(pass)
	}
	// An external test package (x_test) follows the rules of x.
	// The generated test main (x.test) imports only test packages; skip it.
	pkgPath := strings.TrimSuffix(pass.Pkg.Path(), "_test")
	if pkgPath == rules.MainPackage || strings.HasSuffix(pkgPath, ".test") || !strings.HasPrefix(pkgPath, rules.InternalImportPrefix) {
		return nil, nil
	}
	allowed, known := rules.AllowedImports[pkgPath]
	if !known {
		reportUnknownPackage(pass, pkgPath)
		return nil, nil
	}
	for _, file := range pass.Files {
		checkImports(pass, file, pkgPath, allowed)
	}
	return nil, nil
}

// reportUnknownPackage flags an internal package with no C10 rule, so a new
// package cannot escape the layering check by being absent from the table.
func reportUnknownPackage(pass *analysis.Pass, pkgPath string) {
	if len(pass.Files) == 0 {
		return
	}
	pos := pass.Files[0].Package
	if !suppress.Check(pass, "layering", pos) {
		pass.Reportf(pos, "layering: no C10 layering rule for package %s", pkgPath)
	}
}

func checkImports(pass *analysis.Pass, file *ast.File, pkgPath string, allowed map[string]bool) {
	isTest := isTestFile(pass, file)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(path, rules.InternalImportPrefix) {
			continue
		}
		// A test may always import the package it tests (external test packages).
		if allowed[path] || isTest && (path == pkgPath || rules.TestOnlyImports[path] || rules.TestOnlyImportsFor[pkgPath][path]) {
			continue
		}
		reportImport(pass, spec.Pos(), pkgPath, path)
	}
}

func reportImport(pass *analysis.Pass, pos token.Pos, pkgPath, path string) {
	if !suppress.Check(pass, "layering", pos) {
		pass.Reportf(pos, "layering: %s may not import %s", pkgPath, path)
	}
}

func runTools(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		checkToolImports(pass, file)
	}
	return nil, nil
}

func checkToolImports(pass *analysis.Pass, file *ast.File) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(path, rules.InternalImportPrefix) || toolsImportAllowed(pass.Pkg.Path(), path) {
			continue
		}
		if !suppress.Check(pass, "layering", spec.Pos()) {
			pass.Reportf(spec.Pos(), "layering: tools package %s may not import %s", pass.Pkg.Path(), path)
		}
	}
}

func toolsImportAllowed(packagePath, importPath string) bool {
	return strings.HasPrefix(packagePath, rules.DocscheckPrefix) && importPath == rules.DocscheckAPIImport
}

func isTestFile(pass *analysis.Pass, file *ast.File) bool {
	filename := pass.Fset.Position(file.Pos()).Filename
	return strings.HasSuffix(filename, "_test.go")
}
