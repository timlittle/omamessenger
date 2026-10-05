// Package dip checks constructor parameters for concrete internal dependencies.
package dip

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/timlittle/omamessenger/tools/omalint/rules"
	"github.com/timlittle/omamessenger/tools/omalint/suppress"
)

var Analyzer = &analysis.Analyzer{
	Name: "dip",
	Doc:  "require exported internal constructors to depend on interfaces or domain values",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	suppress.Validate(pass)
	if !strings.HasPrefix(pass.Pkg.Path(), rules.InternalImportPrefix) {
		return nil, nil
	}
	for _, file := range pass.Files {
		if !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			checkFile(pass, file)
		}
	}
	return nil, nil
}

func checkFile(pass *analysis.Pass, file *ast.File) {
	for _, decl := range file.Decls {
		fn, ok := constructor(decl, pass)
		if ok {
			checkConstructor(pass, fn)
		}
	}
}

func constructor(decl ast.Decl, pass *analysis.Pass) (*ast.FuncDecl, bool) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Recv != nil || !fn.Name.IsExported() || !strings.HasPrefix(fn.Name.Name, "New") {
		return nil, false
	}
	if _, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func); !ok {
		return nil, false
	}
	return fn, true
}

func checkConstructor(pass *analysis.Pass, fn *ast.FuncDecl) {
	obj := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return
	}
	for i := 0; i < sig.Params().Len(); i++ {
		checkParameter(pass, fn, sig.Params().At(i))
	}
}

func checkParameter(pass *analysis.Pass, fn *ast.FuncDecl, param *types.Var) {
	pkgPath, isDomainValue, isInterface := dependencyShape(param.Type())
	if !strings.HasPrefix(pkgPath, rules.InternalImportPrefix) || pkgPath == pass.Pkg.Path() || isInterface || isDomainValue {
		return
	}
	if !suppress.Check(pass, "dip", fn.Pos()) {
		pass.Reportf(param.Pos(), "dip: constructor %s takes concrete internal dependency %s", fn.Name.Name, param.Type())
	}
}

func dependencyShape(typ types.Type) (pkgPath string, domainValue, isInterface bool) {
	if _, pointer := types.Unalias(typ).(*types.Pointer); pointer {
		return packageOf(types.Unalias(typ).(*types.Pointer).Elem()), false, false
	}
	typ = types.Unalias(typ)
	if iface, ok := typ.Underlying().(*types.Interface); ok {
		return packageOf(typ), false, iface.IsMethodSet()
	}
	path := packageOf(typ)
	return path, strings.HasSuffix(path, "/domain") && isNamed(typ), false
}

func packageOf(typ types.Type) string {
	if named, ok := typ.(*types.Named); ok && named.Obj().Pkg() != nil {
		return named.Obj().Pkg().Path()
	}
	return ""
}

func isNamed(typ types.Type) bool {
	_, ok := typ.(*types.Named)
	return ok
}
