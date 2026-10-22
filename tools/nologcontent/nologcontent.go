// Command nologcontent reports private messaging data reaching logs or the
// process's own output: values of the domain types below, or their content
// fields, passed to log, log/slog, fmt.Print* or fmt.Fprint* to os.Stdout or
// os.Stderr. It is type-aware, which pattern-based linters are not, and it
// has no suppression comment: the privacy rule has no exceptions. It also
// reports a recovered panic value reaching one of those calls formatted
// with anything but %T: Go lets any value be panicked with, so %v or %s
// on one could format a connector's or the UI's own request data just as
// easily as it could a plain string (see errors.md and
// server.PanicLogLine).
package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strconv"
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

// main runs the analyzer as a standalone command.
func main() { singlechecker.Main(Analyzer) }

// run checks every call in the package.
func run(pass *analysis.Pass) (any, error) {
	recovered := recoveredObjects(pass)

	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				checkCall(pass, call, recovered)
			}

			return true
		})
	}

	return nil, nil
}

// checkCall reports each sensitive argument of a call that writes to a log
// or the process's output, and, for any fmt or log call regardless of
// where it writes (checkRecoveredFormatting's own comment says why),
// a recovered panic value among its arguments not formatted with %T.
func checkCall(pass *analysis.Pass, call *ast.CallExpr, recovered map[types.Object]bool) {
	pkgPath, name := calledFunction(pass, call.Fun)

	if firstContent, isOutput := contentStart(pass, pkgPath, name, call.Args); isOutput {
		for _, arg := range call.Args[firstContent:] {
			if sensitiveExpr(pass, arg) {
				pass.Reportf(arg.Pos(), "nologcontent: sensitive messaging data passed to %s.%s", pkgPath, name)
			}
		}
	}

	if pkgPath == "fmt" || pkgPath == "log" {
		checkRecoveredFormatting(pass, call, pkgPath, name, recovered)
	}
}

// recoveredObjects collects every local variable declared or assigned
// directly from a call to the builtin recover, across every file in the
// pass: the one value checkRecoveredFormatting treats as unsafe to log
// with anything but %T.
func recoveredObjects(pass *analysis.Pass) map[types.Object]bool {
	objs := map[types.Object]bool{}

	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}

			for i, rhs := range assign.Rhs {
				if isRecoverCall(pass, rhs) && i < len(assign.Lhs) {
					markRecovered(pass, assign.Lhs[i], objs)
				}
			}

			return true
		})
	}

	return objs
}

// isRecoverCall reports whether expr calls the builtin recover.
func isRecoverCall(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}

	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}

	builtin, ok := pass.TypesInfo.Uses[ident].(*types.Builtin)
	return ok && builtin.Name() == "recover"
}

// markRecovered records lhs's own object as holding a recovered value,
// when lhs is a plain identifier other than "_", which keeps nothing.
func markRecovered(pass *analysis.Pass, lhs ast.Expr, objs map[types.Object]bool) {
	ident, ok := lhs.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return
	}

	if obj := pass.TypesInfo.ObjectOf(ident); obj != nil {
		objs[obj] = true
	}
}

// checkRecoveredFormatting reports a recovered panic value passed
// directly to call, a fmt or log function, unless the format string's
// own literal verb for that argument's position is exactly %T: every
// other verb, including %v and %s, could format whatever the panic
// actually carried, which this analyzer has no way to know is safe.
// Checked for every fmt or log call, not only one that writes to a log
// or the process's own output (contentStart's own narrower rule for
// the sensitive-messaging-data check above): fmt.Sprintf or
// fmt.Errorf never writes anywhere themselves, but whatever they
// build from a recovered value with the wrong verb typically reaches
// a log immediately afterwards anyway (see server.PanicLogLine's own
// safe use of this pattern), and there is no safe verb to allow either
// way. A dynamic format string, or a Print-family call with no format
// string at all (which still formats every argument with %v), is
// never considered safe either.
func checkRecoveredFormatting(pass *analysis.Pass, call *ast.CallExpr, pkgPath, name string, recovered map[types.Object]bool) {
	format, formatIndex := formatLiteral(call.Args)

	ordinal := 0
	for i := range call.Args {
		if i == formatIndex {
			continue
		}

		ordinal++
		if !isRecoveredArg(pass, call.Args[i], recovered) {
			continue
		}

		if format == "" || verbAt(format, ordinal) != "T" {
			pass.Reportf(call.Args[i].Pos(), "nologcontent: a recovered panic value reached %s.%s formatted with anything but %%T", pkgPath, name)
		}
	}
}

// isRecoveredArg reports whether arg is a bare identifier resolving to
// one of recovered's own objects.
func isRecoveredArg(pass *analysis.Pass, arg ast.Expr, recovered map[types.Object]bool) bool {
	ident, ok := arg.(*ast.Ident)
	if !ok {
		return false
	}

	obj := pass.TypesInfo.ObjectOf(ident)
	return obj != nil && recovered[obj]
}

// formatLiteral returns the first string literal among args and its own
// index, the format string a Printf-family call's later arguments are
// read against, or ("", -1) when none of args is a string literal.
func formatLiteral(args []ast.Expr) (string, int) {
	for i, arg := range args {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}

		if s, err := strconv.Unquote(lit.Value); err == nil {
			return s, i
		}
	}

	return "", -1
}

// verbPattern matches one fmt conversion verb, including its flags,
// width and precision, but not the "%%" that formats a literal percent
// sign and consumes no argument (handled separately by verbAt).
var verbPattern = regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)

// verbAt returns the letter of format's nth conversion verb (1-indexed,
// "%%" not counted, since it consumes no argument), or "" once n runs
// past how many format actually has.
func verbAt(format string, n int) string {
	count := 0
	for _, m := range verbPattern.FindAllString(format, -1) {
		verb := m[len(m)-1:]
		if verb == "%" {
			continue
		}

		count++
		if count == n {
			return verb
		}
	}

	return ""
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

// calledFunction returns the package path and name of the function a call
// expression calls, as far as it can tell.
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

// standardOutput reports whether expr is os.Stdout or os.Stderr.
func standardOutput(pass *analysis.Pass, expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Stdout" && selector.Sel.Name != "Stderr" {
		return false
	}

	obj, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Var)
	return ok && obj.Pkg() != nil && obj.Pkg().Path() == "os"
}

// sensitiveExpr reports whether expr contains a sensitive value anywhere.
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

// sensitiveNode reports whether node is a sensitive value or a content
// field of one.
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
