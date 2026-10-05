package archtest

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

type methodUse struct {
	name   string
	recv   string
	fields map[string]bool
	calls  map[string]bool
}

func TestInstabilityMetric(t *testing.T) {
	tests := []struct {
		ca, ce int
		want   float64
	}{{0, 0, 0}, {0, 3, 1}, {3, 0, 0}, {2, 2, 0.5}}
	for _, test := range tests {
		if got := instability(test.ca, test.ce); got != test.want {
			t.Errorf("instability(%d,%d) = %v, want %v", test.ca, test.ce, got, test.want)
		}
	}
}

func TestTestSupportPackageException(t *testing.T) {
	for path, want := range map[string]bool{
		"github.com/timlittle/omamessenger/backend/internal/connector/clocktest":     true,
		"github.com/timlittle/omamessenger/backend/internal/connector/connectortest": true,
		"github.com/timlittle/omamessenger/backend/internal/connector":               false,
	} {
		if got := isTestSupportPackage(path); got != want {
			t.Errorf("isTestSupportPackage(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestMetricLCOM4Synthetic(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "backend", "internal", "archtest", "testdata", "lcom")
	fset := token.NewFileSet()
	parsed, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool { return strings.HasSuffix(info.Name(), ".go") }, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	files := parsed["synthetic"].Files
	var syntax []*ast.File
	for _, file := range files {
		syntax = append(syntax, file)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	checked, err := (&types.Config{Importer: importer.Default()}).Check("synthetic", fset, syntax, info)
	if err != nil {
		t.Fatal(err)
	}
	pkg := &packages.Package{PkgPath: "synthetic", Fset: fset, Syntax: syntax, Types: checked, TypesInfo: info}
	if got := lcom4(pkg, "Sample"); got != 2 {
		t.Fatalf("LCOM4(Sample) = %d, want 2", got)
	}
	if got := methodCount(pkg, "Sample"); got != 3 {
		t.Fatalf("Sample method count = %d, want 3", got)
	}
}

func isStruct(pkg *packages.Package, spec *ast.TypeSpec) bool {
	obj, ok := pkg.TypesInfo.Defs[spec.Name].(*types.TypeName)
	if !ok {
		return false
	}
	_, ok = obj.Type().Underlying().(*types.Struct)
	return ok
}

func methodCount(pkg *packages.Package, typeName string) int {
	return len(methodsFor(pkg, typeName))
}

func lcom4(pkg *packages.Package, typeName string) int {
	methods := methodsFor(pkg, typeName)
	if len(methods) == 0 {
		return 0
	}
	parents := make([]int, len(methods))
	for i := range parents {
		parents[i] = i
	}
	for i := range methods {
		for j := i + 1; j < len(methods); j++ {
			if shareField(methods[i], methods[j]) || callsMethod(methods[i], methods[j].name) || callsMethod(methods[j], methods[i].name) {
				union(parents, i, j)
			}
		}
	}
	components := map[int]bool{}
	for i := range parents {
		components[find(parents, i)] = true
	}
	return len(components)
}

func methodsFor(pkg *packages.Package, typeName string) []methodUse {
	var methods []methodUse
	for _, file := range pkg.Syntax {
		if isTestFile(pkg, file) {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || len(fn.Recv.List) == 0 {
				continue
			}
			recvType := pkg.TypesInfo.TypeOf(fn.Recv.List[0].Type)
			named := namedType(recvType)
			if named == nil || named.Obj().Name() != typeName {
				continue
			}
			receiverName := ""
			if len(fn.Recv.List[0].Names) > 0 {
				receiverName = fn.Recv.List[0].Names[0].Name
			}
			method := methodUse{name: fn.Name.Name, recv: receiverName, fields: map[string]bool{}, calls: map[string]bool{}}
			collectMethodUses(pkg, fn.Body, &method)
			methods = append(methods, method)
		}
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].name < methods[j].name })
	return methods
}

func collectMethodUses(pkg *packages.Package, body *ast.BlockStmt, method *methodUse) {
	ast.Inspect(body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || receiverRoot(selector.X) != method.recv {
			return true
		}
		selection := pkg.TypesInfo.Selections[selector]
		if selection == nil {
			return true
		}
		switch selection.Kind() {
		case types.FieldVal:
			method.fields[selection.Obj().Name()] = true
		case types.MethodVal, types.MethodExpr:
			method.calls[selection.Obj().Name()] = true
		}
		return true
	})
}

func receiverRoot(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return receiverRoot(value.X)
	case *ast.StarExpr:
		return receiverRoot(value.X)
	case *ast.IndexExpr:
		return receiverRoot(value.X)
	default:
		return ""
	}
}

func namedType(typ types.Type) *types.Named {
	switch value := types.Unalias(typ).(type) {
	case *types.Pointer:
		return namedType(value.Elem())
	case *types.Named:
		return value
	default:
		return nil
	}
}

func shareField(left, right methodUse) bool {
	for field := range left.fields {
		if right.fields[field] {
			return true
		}
	}
	return false
}

func callsMethod(method methodUse, name string) bool { return method.calls[name] }

func find(parents []int, index int) int {
	if parents[index] != index {
		parents[index] = find(parents, parents[index])
	}
	return parents[index]
}

func union(parents []int, left, right int) {
	leftRoot, rightRoot := find(parents, left), find(parents, right)
	if leftRoot != rightRoot {
		parents[rightRoot] = leftRoot
	}
}

func lcomSummary(pkg *packages.Package) string {
	var values []string
	for _, file := range pkg.Syntax {
		if isTestFile(pkg, file) {
			continue
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && isStruct(pkg, typeSpec) {
					values = append(values, fmt.Sprintf("%s:%d", typeSpec.Name.Name, lcom4(pkg, typeSpec.Name.Name)))
				}
			}
		}
	}
	if len(values) == 0 {
		return "—"
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}
