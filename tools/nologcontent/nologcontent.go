// Command nologcontent reports private messaging data reaching logs or the
// process's own output: values of the domain types below, or their content
// fields, passed to log, log/slog, fmt.Print* or fmt.Fprint* to os.Stdout or
// os.Stderr. It is type-aware, which pattern-based linters are not, and it
// has no suppression comment: the privacy rule has no exceptions.
package main

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

// sensitiveTypes are domain types whose values carry user content.
var sensitiveTypes = map[string]bool{"Account": true, "Contact": true, "Conversation": true, "Message": true}

// sensitiveFields are content-bearing fields of those types.
var sensitiveFields = map[string]bool{
	"Text": true, "Name": true, "SenderName": true, "Title": true,
	"Preview": true, "PreviewSender": true, "Match": true,
}

// Analyzer is the nologcontent check.
var Analyzer = &analysis.Analyzer{
	Name: "nologcontent",
	Doc:  "prevent logging or printing normalized message and contact content",
	Run:  run,
}

func main() { singlechecker.Main(Analyzer) }

func run(pass *analysis.Pass) (any, error) {
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
	firstContent, isOutput := contentStart(pass, pkgPath, name, call.Args)
	if !isOutput {
		return
	}
	for _, arg := range call.Args[firstContent:] {
		if sensitiveExpr(pass, arg) {
			pass.Reportf(arg.Pos(), "nologcontent: sensitive messaging data passed to %s.%s", pkgPath, name)
		}
	}
}

// contentStart reports whether the call writes to a log or the process's
// output, and the index of its first content argument.
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
	if !ok || !sensitiveFields[selector.Sel.Name] {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && selection.Kind() == types.FieldVal && sensitiveType(selection.Recv())
}

// sensitiveType reports whether typ is, points to, or contains a sensitive
// domain type.
func sensitiveType(typ types.Type) bool {
	switch value := types.Unalias(typ).(type) {
	case *types.Map:
		return sensitiveType(value.Key()) || sensitiveType(value.Elem())
	case interface{ Elem() types.Type }: // pointer, slice, array, channel
		return sensitiveType(value.Elem())
	case *types.Named:
		pkg := value.Obj().Pkg()
		return pkg != nil && strings.HasSuffix(pkg.Path(), "/domain") && sensitiveTypes[value.Obj().Name()]
	}
	return false
}
