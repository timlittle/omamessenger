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
	allowed, known := rules.AllowedImports[pass.Pkg.Path()]
	if !known {
		return nil, nil
	}
	for _, file := range pass.Files {
		checkImports(pass, file, allowed, isTestFile(pass, file))
	}
	return nil, nil
}

func checkImports(pass *analysis.Pass, file *ast.File, allowed map[string]bool, isTest bool) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(path, rules.InternalImportPrefix) {
			continue
		}
		if allowed[path] || isTest && rules.TestOnlyImports[path] {
			continue
		}
		reportImport(pass, spec.Pos(), path)
	}
}

func reportImport(pass *analysis.Pass, pos token.Pos, path string) {
	if !suppress.Check(pass, "layering", pos) {
		pass.Reportf(pos, "layering: %s may not import %s", pass.Pkg.Path(), path)
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
	_ = pass
	filename := pass.Fset.Position(file.Pos()).Filename
	return strings.HasSuffix(filename, "_test.go")
}
