// Package frozeniface protects the core connector contracts from accidental growth.
package frozeniface

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/timlittle/omamessenger/tools/omalint/rules"
	"github.com/timlittle/omamessenger/tools/omalint/suppress"
)

var Analyzer = &analysis.Analyzer{
	Name: "frozeniface",
	Doc:  "check frozen connector interface method sets",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	suppress.Validate(pass)
	if pass.Pkg.Name() != rules.FrozenInterfacePackageName {
		return nil, nil
	}
	for _, file := range pass.Files {
		checkFile(pass, file)
	}
	return nil, nil
}

func checkFile(pass *analysis.Pass, file *ast.File) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				checkType(pass, typeSpec)
			}
		}
	}
}

func checkType(pass *analysis.Pass, typeSpec *ast.TypeSpec) {
	expected, frozen := rules.FrozenInterfaces[typeSpec.Name.Name]
	if !frozen {
		return
	}
	obj, ok := pass.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
	if !ok {
		return
	}
	iface, ok := obj.Type().Underlying().(*types.Interface)
	if !ok {
		if !suppress.Check(pass, "frozeniface", typeSpec.Pos()) {
			pass.Reportf(typeSpec.Pos(), "frozeniface: %s must remain an interface", typeSpec.Name.Name)
		}
		return
	}
	got := methodNames(iface)
	if !equal(got, expected) && !suppress.Check(pass, "frozeniface", typeSpec.Pos()) {
		pass.Reportf(typeSpec.Pos(), "frozeniface: %s methods are %v; want %v", typeSpec.Name.Name, got, sorted(expected))
	}
}

func methodNames(iface *types.Interface) []string {
	iface.Complete()
	methods := make([]string, 0, iface.NumMethods())
	for i := 0; i < iface.NumMethods(); i++ {
		methods = append(methods, iface.Method(i).Name())
	}
	sort.Strings(methods)
	return methods
}

func equal(got, want []string) bool {
	return strings.Join(got, "\x00") == strings.Join(sorted(want), "\x00")
}

func sorted(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
