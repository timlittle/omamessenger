// Package markdown extracts the facts docscheck verifies from the project's
// markdown: contract tables, links, repo paths, quoted commands and flags.
// Every function is pure so each can be tested on a small fixture.
package markdown

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Section returns the text from a heading line starting with heading up to
// the next heading of the same or higher level.
func Section(md, heading string) string {
	start := strings.Index(md, "\n"+heading)
	if start < 0 {
		return ""
	}
	body := md[start+1:]
	level := strings.SplitN(heading, " ", 2)[0] + " "
	if end := strings.Index(body[len(heading):], "\n"+level); end >= 0 {
		return body[:len(heading)+end]
	}
	return body
}

var backticked = regexp.MustCompile("`([^`]+)`")

// TableNames returns the first-column names of the markdown tables in md,
// grouped by the first header word of each table (e.g. "method", "event").
func TableNames(md string) map[string][]string {
	names := map[string][]string{}
	kind := ""
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			kind = ""
			continue
		}
		cells := strings.Split(line, "|")
		first := strings.TrimSpace(cells[1])
		switch {
		case kind == "":
			kind = first
		case strings.HasPrefix(first, "`"):
			names[kind] = append(names[kind], backticked.FindStringSubmatch(first)[1])
		}
	}
	return names
}

// LayeringTable parses the C10 layering table into package -> allowed
// imports, as full paths under internalPrefix. Rows for the main package and
// tools are skipped: they are rules, not import lists.
func LayeringTable(md, internalPrefix string) map[string]map[string]bool {
	table := map[string]map[string]bool{}
	for _, line := range strings.Split(md, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || !strings.HasPrefix(strings.TrimSpace(cells[1]), "`backend/internal/") {
			continue
		}
		allowed := map[string]bool{}
		for _, part := range strings.Split(parenthetical.ReplaceAllString(cells[2], ""), ",") {
			if name := strings.Fields(strings.Trim(part, " `—")); len(name) > 0 {
				allowed[internalPrefix+strings.Trim(name[0], "`")] = true
			}
		}
		for _, pkg := range backticked.FindAllStringSubmatch(cells[1], -1) {
			table[internalPrefix+strings.TrimPrefix(pkg[1], "backend/internal/")] = allowed
		}
	}
	return table
}

// parenthetical matches a note such as "(test-only package)" in a table cell.
var parenthetical = regexp.MustCompile(`\([^)]*\)`)

var gateLine = regexp.MustCompile(`^\s*- (?:≥ )?(\d+) %: (.*)$`)

// CoverageGates parses C9 bullets such as "- ≥ 90 %: `store`, `rpc`" into
// package -> threshold. Text after ";" is commentary. `backend` is the main
// package, `tools/*` the tools prefix, and other names sit under
// backend/internal/.
func CoverageGates(md string) map[string]float64 {
	gates := map[string]float64{}
	for _, line := range strings.Split(md, "\n") {
		match := gateLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		min, _ := strconv.ParseFloat(match[1], 64)
		names := strings.SplitN(match[2], ";", 2)[0]
		for _, name := range backticked.FindAllStringSubmatch(names, -1) {
			gates[gatePackage(name[1])] = min
		}
	}
	return gates
}

func gatePackage(name string) string {
	if name == "backend" || strings.HasPrefix(name, "tools/") {
		return name
	}
	return "backend/internal/" + name
}

var linkTarget = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// RelativeLinks returns markdown link targets that point into the repo,
// without any #fragment.
func RelativeLinks(md string) []string {
	var links []string
	for _, match := range linkTarget.FindAllStringSubmatch(md, -1) {
		target := strings.SplitN(match[1], "#", 2)[0]
		if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		links = append(links, target)
	}
	return links
}

var repoPath = regexp.MustCompile("`((?:ui|backend|scripts|tools|tests|docs|bin)/[^`\\s]*)")

// PathRef is a repo path named in a doc. planned paths are on a line that
// says "(planned)" and may not exist yet.
type PathRef struct {
	Path    string
	Planned bool
}

func RepoPaths(md string) []PathRef {
	var refs []PathRef
	for _, line := range strings.Split(md, "\n") {
		planned := strings.Contains(line, "(planned)")
		for _, match := range repoPath.FindAllStringSubmatch(line, -1) {
			refs = append(refs, PathRef{Path: strings.TrimRight(match[1], ".,:;"), Planned: planned})
		}
	}
	return refs
}

// IgnoredDirs returns the root-anchored directories a .gitignore excludes
// ("/bin/dev/" becomes "bin/dev/"). They hold build output, so docs may name
// paths inside them that a clean checkout does not have.
func IgnoredDirs(gitignore string) []string {
	var dirs []string
	for _, line := range strings.Split(gitignore, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/") {
			dirs = append(dirs, strings.TrimPrefix(line, "/"))
		}
	}
	return dirs
}

// PathExists reports whether root/p exists. Patterns with * or {a,b} must
// match at least one file.
func PathExists(root, p string) bool {
	for _, candidate := range expandBraces(p) {
		matches, err := filepath.Glob(filepath.Join(root, candidate))
		if err != nil || len(matches) == 0 {
			return false
		}
	}
	return true
}

func expandBraces(p string) []string {
	open := strings.Index(p, "{")
	end := strings.Index(p, "}")
	if open < 0 || end < open {
		return []string{p}
	}
	var out []string
	for _, choice := range strings.Split(p[open+1:end], ",") {
		out = append(out, expandBraces(p[:open]+choice+p[end+1:])...)
	}
	return out
}

var fence = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")

// CodeLines returns each line of every fenced block and each inline code
// span: the places where docs quote commands rather than prose.
func CodeLines(md string) []string {
	var lines []string
	for _, block := range fence.FindAllStringSubmatch(md, -1) {
		lines = append(lines, strings.Split(block[1], "\n")...)
	}
	for _, span := range backticked.FindAllStringSubmatch(fence.ReplaceAllString(md, ""), -1) {
		lines = append(lines, span[1])
	}
	return lines
}

var makeCall = regexp.MustCompile(`\bmake ([a-z][a-z0-9-]*)`)

// MakeTargets returns targets of `make <target>` commands quoted in md.
func MakeTargets(md string) []string {
	var targets []string
	for _, line := range CodeLines(md) {
		for _, match := range makeCall.FindAllStringSubmatch(line, -1) {
			targets = append(targets, match[1])
		}
	}
	return targets
}

var makefileTarget = regexp.MustCompile(`(?m)^([a-zA-Z0-9_-]+):`)

func MakefileTargets(src string) map[string]bool {
	targets := map[string]bool{}
	for _, match := range makefileTarget.FindAllStringSubmatch(src, -1) {
		targets[match[1]] = true
	}
	return targets
}

var flagName = regexp.MustCompile(`(?:^|\s)--([a-z][a-z-]*)`)

// otherTools marks quoted commands whose flags belong to another program.
var otherTools = []string{"omarchy", "git ", "go build", "go test", "go vet", "node "}

// HelperFlags returns --flags quoted in md, skipping commands of other tools.
func HelperFlags(md string) []string {
	var flags []string
	for _, line := range CodeLines(md) {
		if mentionsAny(line, otherTools) {
			continue
		}
		for _, match := range flagName.FindAllStringSubmatch(line, -1) {
			flags = append(flags, match[1])
		}
	}
	return flags
}

func mentionsAny(line string, words []string) bool {
	for _, word := range words {
		if strings.Contains(line, word) {
			return true
		}
	}
	return false
}

// DefinedFlags returns the flag names a Go file registers with the flag
// package (fs.BoolVar(&v, "name", ...), fs.String("name", ...), ...).
func DefinedFlags(src []byte) (map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "config.go", src, 0)
	if err != nil {
		return nil, err
	}
	flags := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		if name, ok := flagDefinition(node); ok {
			flags[name] = true
		}
		return true
	})
	return flags, nil
}

// flagNameArg is the argument index of the flag name for each flag-package
// definition function.
var flagNameArg = map[string]int{
	"Bool": 0, "String": 0, "Int": 0, "Int64": 0, "Duration": 0,
	"BoolVar": 1, "StringVar": 1, "IntVar": 1, "Int64Var": 1, "DurationVar": 1,
}

func flagDefinition(node ast.Node) (string, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	arg, known := flagNameArg[selector.Sel.Name]
	if !known || len(call.Args) <= arg {
		return "", false
	}
	literal, ok := call.Args[arg].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	name, err := strconv.Unquote(literal.Value)
	return name, err == nil
}

// MarkedBlock returns the text between <!-- name:start --> and
// <!-- name:end -->, and whether both markers are present.
func MarkedBlock(md, name string) (string, bool) {
	start := "<!-- " + name + ":start -->"
	end := "<!-- " + name + ":end -->"
	i, j := strings.Index(md, start), strings.Index(md, end)
	if i < 0 || j < i {
		return "", false
	}
	return strings.TrimSpace(md[i+len(start) : j]), true
}
