// Package main implements covergate, which enforces per-package statement
// coverage from a Go cover profile against the C9 gates.
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// ModulePath prefixes every file in the cover profile.
const ModulePath = "github.com/timlittle/omamessenger/"

// MainFile is the only file whose func main is excluded from coverage.
const MainFile = "backend/main.go"

// Gate is a minimum statement coverage for one package directory.
type Gate struct {
	Package string
	Min     float64
}

// Gates is the single source of the C9 coverage thresholds. docscheck
// verifies that docs/TASKS.md C9 lists the same values.
var Gates = []Gate{
	{"backend", 80},
	{"backend/internal/domain", 100},
	{"backend/internal/app/policy", 100},
	{"backend/internal/store", 90},
	{"backend/internal/rpc", 90},
	{"backend/internal/api", 90},
	{"backend/internal/app", 90},
	{"backend/internal/connector", 90},
	{"backend/internal/connector/demo", 90},
	{"backend/internal/connector/clocktest", 90},
	{"backend/internal/connector/connectortest", 90},
	{"backend/internal/notify", 75},
}

// ToolsPrefix packages share one gate.
const ToolsPrefix = "tools/"

// ToolsMin is the gate for every package under ToolsPrefix.
const ToolsMin = 90

// Count holds statement totals for one package.
type Count struct {
	Statements int
	Covered    int
}

// Percent returns covered statements as a percentage.
func (c Count) Percent() float64 {
	if c.Statements == 0 {
		return 100
	}
	return 100 * float64(c.Covered) / float64(c.Statements)
}

// lineRange is an inclusive range of source lines.
type lineRange struct{ start, end int }

type block struct {
	file       string
	start, end int
	statements int
	count      int
}

// ParseProfile aggregates a cover profile per package directory. Blocks that
// appear more than once (several test binaries) count once, covered if any run
// covered them. Blocks inside exclude are skipped.
func ParseProfile(r io.Reader, exclude lineRange) (map[string]Count, error) {
	seen := map[string]block{}
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "mode:") {
			continue
		}
		b, err := parseBlock(text)
		if err != nil {
			return nil, fmt.Errorf("profile line %d: %w", line, err)
		}
		key := fmt.Sprintf("%s:%d-%d", b.file, b.start, b.end)
		if prev, ok := seen[key]; ok && prev.count > b.count {
			continue
		}
		seen[key] = b
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return aggregate(seen, exclude), nil
}

func aggregate(blocks map[string]block, exclude lineRange) map[string]Count {
	counts := map[string]Count{}
	for _, b := range blocks {
		if b.file == MainFile && b.start >= exclude.start && b.end <= exclude.end {
			continue
		}
		dir := path.Dir(b.file)
		c := counts[dir]
		c.Statements += b.statements
		if b.count > 0 {
			c.Covered += b.statements
		}
		counts[dir] = c
	}
	return counts
}

// parseBlock reads "module/file.go:startLine.col,endLine.col statements count".
func parseBlock(text string) (block, error) {
	colon := strings.LastIndex(text, ":")
	fields := strings.Fields(text[colon+1:])
	if colon < 0 || len(fields) != 3 {
		return block{}, fmt.Errorf("malformed block %q", text)
	}
	start, end, err := parseSpan(fields[0])
	if err != nil {
		return block{}, err
	}
	statements, err1 := strconv.Atoi(fields[1])
	count, err2 := strconv.Atoi(fields[2])
	if err1 != nil || err2 != nil {
		return block{}, fmt.Errorf("malformed counts in %q", text)
	}
	file := strings.TrimPrefix(text[:colon], ModulePath)
	return block{file: file, start: start, end: end, statements: statements, count: count}, nil
}

func parseSpan(span string) (int, int, error) {
	parts := strings.Split(span, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("malformed span %q", span)
	}
	start, err1 := strconv.Atoi(strings.Split(parts[0], ".")[0])
	end, err2 := strconv.Atoi(strings.Split(parts[1], ".")[0])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("malformed span %q", span)
	}
	return start, end, nil
}

// MainRange finds the lines of func main in src. A file without func main
// yields an empty range that excludes nothing.
func MainRange(src []byte) (lineRange, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, MainFile, src, 0)
	if err != nil {
		return lineRange{}, err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name.Name == "main" {
			return lineRange{fset.Position(fn.Pos()).Line, fset.Position(fn.End()).Line}, nil
		}
	}
	return lineRange{start: 1, end: 0}, nil
}

// Result is the outcome for one package.
type Result struct {
	Package string
	Count   Count
	Min     float64
	Problem string
}

// Passed reports whether the package met its gate.
func (r Result) Passed() bool { return r.Problem == "" }

// GateFor returns the threshold for pkg and whether one is defined.
func GateFor(pkg string) (float64, bool) {
	for _, gate := range Gates {
		if gate.Package == pkg {
			return gate.Min, true
		}
	}
	if strings.HasPrefix(pkg, ToolsPrefix) {
		return ToolsMin, true
	}
	return 0, false
}

// Evaluate compares counts with the gates. Packages without statements are
// skipped; a package with statements and no gate fails.
func Evaluate(counts map[string]Count) []Result {
	var results []Result
	for pkg, count := range counts {
		if count.Statements == 0 {
			continue
		}
		min, ok := GateFor(pkg)
		result := Result{Package: pkg, Count: count, Min: min}
		switch {
		case !ok:
			result.Problem = "no gate defined"
		case count.Percent() < min:
			result.Problem = "below gate"
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Package < results[j].Package })
	return results
}

// Render prints results as an aligned table and reports whether all passed.
func Render(w io.Writer, results []Result) bool {
	ok := true
	fmt.Fprintf(w, "%-45s %8s %6s  %s\n", "package", "coverage", "gate", "status")
	for _, r := range results {
		status := "ok"
		if !r.Passed() {
			status, ok = "FAIL: "+r.Problem, false
		}
		fmt.Fprintf(w, "%-45s %7.1f%% %5.0f%%  %s\n", r.Package, r.Count.Percent(), r.Min, status)
	}
	return ok
}
