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
				if comment.End() <= pos && pass.Fset.Position(pos).Line-pass.Fset.Position(comment.Pos()).Line <= 1 {
					ignored = true
				}
			}
		}
	}
	return ignored
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
