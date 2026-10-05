// Package complexity checks cyclomatic complexity and control nesting.
package complexity

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/timlittle/omamessenger/tools/omalint/rules"
	"github.com/timlittle/omamessenger/tools/omalint/suppress"
)

var Analyzer = &analysis.Analyzer{
	Name: "complexity",
	Doc:  "check function complexity and nesting against C10 limits",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	suppress.Validate(pass)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if strings.HasSuffix(filename, "_test.go") || generated(file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch fn := node.(type) {
			case *ast.FuncDecl:
				if fn.Body != nil {
					checkFunction(pass, fn.Pos(), fn.Name.Name, fn.Body)
				}
			case *ast.FuncLit:
				checkFunction(pass, fn.Pos(), "function literal", fn.Body)
			}
			return true
		})
	}
	return nil, nil
}

func checkFunction(pass *analysis.Pass, pos token.Pos, name string, body *ast.BlockStmt) {
	complexity, maxDepth := metrics(body)
	if complexity > rules.MaxComplexity && !suppress.Check(pass, "complexity", pos) {
		pass.Reportf(pos, "complexity: %s has complexity %d; maximum is %d", name, complexity, rules.MaxComplexity)
	}
	if maxDepth > rules.MaxNestingDepth && !suppress.Check(pass, "complexity", pos) {
		pass.Reportf(pos, "complexity: %s has nesting depth %d; maximum is %d", name, maxDepth, rules.MaxNestingDepth)
	}
}

func metrics(body *ast.BlockStmt) (complexity, maxDepth int) {
	counter := metricCounter{complexity: 1}
	ast.Inspect(body, counter.visit)
	return counter.complexity, counter.maxDepth
}

type metricCounter struct {
	complexity   int
	currentDepth int
	maxDepth     int
	active       []bool
}

func (counter *metricCounter) visit(node ast.Node) bool {
	if node == nil {
		counter.leave()
		return true
	}
	if _, nested := node.(*ast.FuncLit); nested {
		counter.active = append(counter.active, false)
		return false
	}
	counter.enter(isControl(node))
	counter.countDecision(node)
	return true
}

func (counter *metricCounter) leave() {
	if len(counter.active) == 0 {
		return
	}
	if counter.active[len(counter.active)-1] {
		counter.currentDepth--
	}
	counter.active = counter.active[:len(counter.active)-1]
}

func (counter *metricCounter) enter(control bool) {
	counter.active = append(counter.active, control)
	if !control {
		return
	}
	counter.currentDepth++
	if counter.currentDepth > counter.maxDepth {
		counter.maxDepth = counter.currentDepth
	}
}

func (counter *metricCounter) countDecision(node ast.Node) {
	switch n := node.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
		counter.complexity++
	case *ast.CaseClause:
		if n.List != nil {
			counter.complexity++
		}
	case *ast.CommClause:
		counter.complexity++
	case *ast.BinaryExpr:
		if n.Op == token.LAND || n.Op == token.LOR {
			counter.complexity++
		}
	}
}

func isControl(node ast.Node) bool {
	switch node.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return true
	default:
		return false
	}
}

func generated(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if comment.Pos() > file.Package {
				break
			}
			if strings.HasPrefix(comment.Text, "// Code generated ") {
				return true
			}
		}
	}
	return false
}
