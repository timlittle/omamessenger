// Package nologcontent prevents private messaging data from reaching logs/output.
package nologcontent

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/timlittle/omamessenger/tools/omalint/rules"
	"github.com/timlittle/omamessenger/tools/omalint/suppress"
)

var Analyzer = &analysis.Analyzer{
	Name: "nologcontent",
	Doc:  "prevent logging or printing normalized message and contact content",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	suppress.Validate(pass)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				checkCall(pass, call)
			}
			return true
		})
	}
	return nil, nil
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	pkgPath, name := calledFunction(pass, call.Fun)
	firstContent, allowed := contentStart(pass, pkgPath, name, call.Args)
	if !allowed {
		return
	}
	for _, arg := range call.Args[firstContent:] {
		if sensitiveExpr(pass, arg) && !suppress.Check(pass, "nologcontent", call.Pos()) {
			pass.Reportf(arg.Pos(), "nologcontent: sensitive messaging data passed to %s.%s", pkgPath, name)
		}
	}
}

func contentStart(pass *analysis.Pass, pkgPath, name string, args []ast.Expr) (int, bool) {
	if pkgPath == "log" || pkgPath == "log/slog" || pkgPath == "fmt" && strings.HasPrefix(name, "Print") {
		return 0, true
	}
	if pkgPath != "fmt" || !strings.HasPrefix(name, "Fprint") || len(args) == 0 || !standardOutput(pass, args[0]) {
		return 0, false
	}
	return 1, true
}

func calledFunction(pass *analysis.Pass, fun ast.Expr) (pkgPath, name string) {
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	name = selector.Sel.Name
	if selection := pass.TypesInfo.Selections[selector]; selection != nil {
		if fn, ok := selection.Obj().(*types.Func); ok && fn.Pkg() != nil {
			return fn.Pkg().Path(), name
		}
	}
	if fn, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Func); ok && fn.Pkg() != nil {
		return fn.Pkg().Path(), name
	}
	return "", name
}

func standardOutput(pass *analysis.Pass, expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Stdout" && selector.Sel.Name != "Stderr" {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Var)
	return ok && obj.Pkg() != nil && obj.Pkg().Path() == "os"
}

func sensitiveExpr(pass *analysis.Pass, expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if node != nil && sensitiveNode(pass, node) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func sensitiveNode(pass *analysis.Pass, node ast.Node) bool {
	if expression, ok := node.(ast.Expr); ok && sensitiveType(pass.TypesInfo.TypeOf(expression)) {
		return true
	}
	selector, ok := node.(*ast.SelectorExpr)
	if !ok || !rules.SensitiveDomainFields[selector.Sel.Name] {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && selection.Kind() == types.FieldVal && domainType(selection.Recv())
}

func sensitiveType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	switch value := types.Unalias(typ).(type) {
	case *types.Pointer:
		return sensitiveType(value.Elem())
	case *types.Slice:
		return sensitiveType(value.Elem())
	case *types.Array:
		return sensitiveType(value.Elem())
	case *types.Map:
		return sensitiveType(value.Key()) || sensitiveType(value.Elem())
	case *types.Named:
		return value.Obj().Pkg() != nil && strings.HasSuffix(value.Obj().Pkg().Path(), "/domain") && rules.SensitiveDomainTypes[value.Obj().Name()]
	default:
		return false
	}
}

func domainType(typ types.Type) bool {
	switch value := types.Unalias(typ).(type) {
	case *types.Pointer:
		return domainType(value.Elem())
	case *types.Named:
		return value.Obj().Pkg() != nil && strings.HasSuffix(value.Obj().Pkg().Path(), "/domain") && rules.SensitiveDomainTypes[value.Obj().Name()]
	default:
		return false
	}
}
