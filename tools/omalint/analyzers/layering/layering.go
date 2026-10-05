// Package layering checks internal package import edges.
package layering

import (
	"go/ast"
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
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil || len(path) < len(rules.InternalImportPrefix) || path[:len(rules.InternalImportPrefix)] != rules.InternalImportPrefix {
				continue
			}
			if allowed[path] || (isTestFile(pass, file) && rules.TestOnlyImports[path]) {
				continue
			}
			if suppress.Check(pass, "layering", spec.Pos()) {
				continue
			}
			pass.Reportf(spec.Pos(), "layering: %s may not import %s", pass.Pkg.Path(), path)
		}
	}
	return nil, nil
}

func runTools(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !strings.HasPrefix(path, rules.InternalImportPrefix) {
				continue
			}
			allowed := strings.HasPrefix(pass.Pkg.Path(), rules.DocscheckPrefix) && path == rules.DocscheckAPIImport
			if !allowed && !suppress.Check(pass, "layering", spec.Pos()) {
				pass.Reportf(spec.Pos(), "layering: tools package %s may not import %s", pass.Pkg.Path(), path)
			}
		}
	}
	return nil, nil
}

func isTestFile(pass *analysis.Pass, file *ast.File) bool {
	_ = pass
	filename := pass.Fset.Position(file.Pos()).Filename
	return strings.HasSuffix(filename, "_test.go")
}
