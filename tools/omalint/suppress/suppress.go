// Package suppress implements the shared omalint suppression syntax.
package suppress

import (
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Validate reports suppression directives that omit their required reason.
func Validate(pass *analysis.Pass) {
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				_, directive, valid := parse(comment.Text)
				if directive && !valid {
					pass.Reportf(comment.Pos(), "suppression requires //omalint:ignore <rule> <reason>")
				}
			}
		}
	}
}

// Check reports malformed suppressions and returns whether pos is suppressed for rule.
func Check(pass *analysis.Pass, rule string, pos token.Pos) bool {
	ignored := false
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				name, directive, valid := parse(comment.Text)
				if !directive || !valid || name != rule {
					continue
				}
				if covers(pass.Fset, comment.Pos(), comment.End(), pos) {
					ignored = true
				}
			}
		}
	}
	return ignored
}

// covers reports whether a directive spanning [start, end) applies to pos: the
// same file, ending before pos, on pos's line or the line directly above.
func covers(fset *token.FileSet, start, end, pos token.Pos) bool {
	if fset.File(start) != fset.File(pos) || end > pos {
		return false
	}
	distance := fset.Position(pos).Line - fset.Position(start).Line
	return distance == 0 || distance == 1
}

func parse(comment string) (rule string, directive, valid bool) {
	text := strings.TrimSpace(strings.TrimPrefix(comment, "//"))
	if !strings.HasPrefix(text, "omalint:ignore") {
		return "", false, false
	}
	if i := strings.Index(text, "// want"); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	fields := strings.Fields(text)
	if len(fields) < 3 || fields[0] != "omalint:ignore" {
		return "", true, false
	}
	return fields[1], true, true
}
