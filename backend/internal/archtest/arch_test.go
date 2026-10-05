package archtest

import (
	"flag"
	"fmt"
	"go/ast"
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

const modulePrefix = "github.com/timlittle/omamessenger/"
const internalPrefix = modulePrefix + "backend/internal/"

var updateArchitecture = flag.Bool("update", false, "update docs/ARCHITECTURE.md")

type packageGraph struct {
	packages map[string]*packages.Package
	imports  map[string]map[string]bool
	ca       map[string]int
	ce       map[string]int
}

func TestEfferentCoupling(t *testing.T) {
	graph := loadGraph(t)
	for _, name := range graph.names() {
		if name != modulePrefix+"backend" && graph.ce[name] > 4 {
			t.Errorf("%s has Ce=%d; maximum is 4", shortName(name), graph.ce[name])
		}
	}
}

func TestStableDependencies(t *testing.T) {
	graph := loadGraph(t)
	for _, source := range graph.names() {
		for target := range graph.imports[source] {
			if instability(graph.ca[target], graph.ce[target]) > instability(graph.ca[source], graph.ce[source]) {
				t.Errorf("unstable edge %s -> %s: I(source)=%.3f I(target)=%.3f", shortName(source), shortName(target), instability(graph.ca[source], graph.ce[source]), instability(graph.ca[target], graph.ce[target]))
			}
		}
	}
}

func TestNoJunkDrawerPackages(t *testing.T) {
	for _, pkg := range loadGraph(t).names() {
		base := strings.ToLower(filepath.Base(pkg))
		switch base {
		case "util", "utils", "common", "helpers", "misc", "shared", "base":
			t.Errorf("forbidden package name %q at %s", base, pkg)
		}
	}
}

func TestLCOM4(t *testing.T) {
	for _, pkg := range loadGraph(t).packages {
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
					if !ok || !isStruct(pkg, typeSpec) {
						continue
					}
					count := lcom4(pkg, typeSpec.Name.Name)
					methods := methodCount(pkg, typeSpec.Name.Name)
					if methods >= 3 && count != 1 {
						t.Errorf("%s.%s has LCOM4=%d across %d methods; expected 1", shortName(pkg.PkgPath), typeSpec.Name.Name, count, methods)
					}
				}
			}
		}
	}
}

func TestExportsUsedOutsidePackage(t *testing.T) {
	allPackages := loadAllPackages(t)
	used := make(map[string]bool)
	for _, pkg := range allPackages {
		if isTestPackage(pkg) {
			continue
		}
		for ident, obj := range pkg.TypesInfo.Uses {
			if strings.HasSuffix(pkg.Fset.Position(ident.Pos()).Filename, "_test.go") {
				continue
			}
			if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() != pkg.PkgPath {
				markObjectUse(obj, used, make(map[types.Type]bool))
			}
		}
	}
	for _, pkg := range allPackages {
		if !strings.HasPrefix(pkg.PkgPath, internalPrefix) || isTestPackage(pkg) || isTestSupportPackage(pkg.PkgPath) {
			continue
		}
		for _, name := range pkg.Types.Scope().Names() {
			obj := pkg.Types.Scope().Lookup(name)
			if obj.Exported() && !used[obj.Pkg().Path()+"."+obj.Name()] {
				t.Errorf("%s.%s is exported but unused outside its package", shortName(pkg.PkgPath), name)
			}
		}
	}
}

func markObjectUse(obj types.Object, used map[string]bool, seen map[types.Type]bool) {
	if obj.Pkg() != nil {
		used[obj.Pkg().Path()+"."+obj.Name()] = true
	}
	markTypeUse(obj.Type(), used, seen)
}

func markTypeUse(typ types.Type, used map[string]bool, seen map[types.Type]bool) {
	if typ == nil || seen[typ] {
		return
	}
	seen[typ] = true
	switch value := types.Unalias(typ).(type) {
	case *types.Named:
		if obj := value.Obj(); obj.Pkg() != nil {
			used[obj.Pkg().Path()+"."+obj.Name()] = true
		}
		markTypeUse(value.Underlying(), used, seen)
	case *types.Pointer:
		markTypeUse(value.Elem(), used, seen)
	case *types.Slice:
		markTypeUse(value.Elem(), used, seen)
	case *types.Array:
		markTypeUse(value.Elem(), used, seen)
	case *types.Map:
		markTypeUse(value.Key(), used, seen)
		markTypeUse(value.Elem(), used, seen)
	case *types.Chan:
		markTypeUse(value.Elem(), used, seen)
	case *types.Tuple:
		for i := 0; i < value.Len(); i++ {
			markTypeUse(value.At(i).Type(), used, seen)
		}
	case *types.Signature:
		if receiver := value.Recv(); receiver != nil {
			markTypeUse(receiver.Type(), used, seen)
		}
		markTypeUse(value.Params(), used, seen)
		markTypeUse(value.Results(), used, seen)
	case *types.Struct:
		for i := 0; i < value.NumFields(); i++ {
			markObjectUse(value.Field(i), used, seen)
		}
	case *types.Interface:
		value.Complete()
		for i := 0; i < value.NumMethods(); i++ {
			markObjectUse(value.Method(i), used, seen)
		}
	case *types.TypeParam:
		markTypeUse(value.Constraint(), used, seen)
	}
}

func TestSuppressionBudget(t *testing.T) {
	root := repoRoot(t)
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "testdata" || entry.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.HasPrefix(strings.TrimSpace(comment.Text), "//omalint:ignore") {
					count++
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count > 5 {
		t.Errorf("found %d omalint suppressions; maximum is 5", count)
	}
}

func TestArchitectureDocCurrent(t *testing.T) {
	graph := loadGraph(t)
	want := architectureDoc(graph)
	path := filepath.Join(repoRoot(t), "docs", "ARCHITECTURE.md")
	if *updateArchitecture {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("docs/ARCHITECTURE.md is stale; run go test -mod=vendor ./backend/internal/archtest -args -update")
	}
}

func loadGraph(t *testing.T) *packageGraph {
	t.Helper()
	loaded := loadAllPackages(t)
	graph := &packageGraph{packages: map[string]*packages.Package{}, imports: map[string]map[string]bool{}, ca: map[string]int{}, ce: map[string]int{}}
	for _, pkg := range loaded {
		if strings.HasPrefix(pkg.PkgPath, internalPrefix) || pkg.PkgPath == modulePrefix+"backend" {
			graph.packages[pkg.PkgPath] = pkg
			graph.imports[pkg.PkgPath] = make(map[string]bool)
		}
	}
	for name, pkg := range graph.packages {
		for imported := range pkg.Imports {
			if _, ok := graph.packages[imported]; ok {
				graph.imports[name][imported] = true
				graph.ce[name]++
				graph.ca[imported]++
			}
		}
	}
	return graph
}

func loadAllPackages(t *testing.T) []*packages.Package {
	t.Helper()
	config := &packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes, Dir: repoRoot(t)}
	pkgs, err := packages.Load(config, "./backend/...")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			t.Errorf("load %s: %v", pkg.PkgPath, pkg.Errors)
		}
	}
	return pkgs
}

func (graph *packageGraph) names() []string {
	names := make([]string, 0, len(graph.packages))
	for name := range graph.packages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func instability(ca, ce int) float64 {
	if ca+ce == 0 {
		return 0
	}
	return float64(ce) / float64(ca+ce)
}

func shortName(path string) string {
	if path == modulePrefix+"backend" {
		return "backend (main)"
	}
	return strings.TrimPrefix(path, internalPrefix)
}

func isTestSupportPackage(path string) bool {
	name := filepath.Base(path)
	return name == "clocktest" || name == "connectortest"
}

func isTestFile(pkg *packages.Package, file *ast.File) bool {
	return strings.HasSuffix(pkg.Fset.Position(file.Pos()).Filename, "_test.go")
}

func isTestPackage(pkg *packages.Package) bool { return strings.Contains(pkg.ID, ".test") }

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func architectureDoc(graph *packageGraph) string {
	var builder strings.Builder
	builder.WriteString("# Backend architecture\n\nGenerated from `backend/internal/archtest`. Run `go test -mod=vendor ./backend/internal/archtest -args -update` after changing package dependencies or structs.\n\n")
	builder.WriteString("```mermaid\ngraph TD\n")
	ids := make(map[string]string)
	for index, name := range graph.names() {
		ids[name] = fmt.Sprintf("p%d", index)
		fmt.Fprintf(&builder, "  %s[\"%s\"]\n", ids[name], shortName(name))
	}
	for _, source := range graph.names() {
		for _, target := range sortedSet(graph.imports[source]) {
			fmt.Fprintf(&builder, "  %s --> %s\n", ids[source], ids[target])
		}
	}
	builder.WriteString("```\n\n| package | Ca | Ce | I | A | distance | LCOM4 |\n|---|---:|---:|---:|---:|---:|---|\n")
	for _, name := range graph.names() {
		pkg := graph.packages[name]
		abstractness := abstractness(pkg)
		instabilityValue := instability(graph.ca[name], graph.ce[name])
		fmt.Fprintf(&builder, "| `%s` | %d | %d | %.2f | %.2f | %.2f | %s |\n", shortName(name), graph.ca[name], graph.ce[name], instabilityValue, abstractness, abs(abstractness+instabilityValue-1), lcomSummary(pkg))
	}
	return builder.String()
}

func sortedSet(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func abstractness(pkg *packages.Package) float64 {
	if pkg.Types == nil {
		return 0
	}
	total, abstract := 0, 0
	for _, name := range pkg.Types.Scope().Names() {
		if !token.IsExported(name) {
			continue
		}
		typename, ok := pkg.Types.Scope().Lookup(name).(*types.TypeName)
		if !ok || types.Unalias(typename.Type()) != typename.Type() {
			continue
		}
		named, ok := typename.Type().(*types.Named)
		if !ok {
			continue
		}
		total++
		if _, ok := named.Underlying().(*types.Interface); ok {
			abstract++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(abstract) / float64(total)
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
