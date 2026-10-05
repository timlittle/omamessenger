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
				text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
				if i := strings.Index(text, "// want"); i >= 0 {
					text = strings.TrimSpace(text[:i])
				}
				if !strings.HasPrefix(text, "omalint:ignore") {
					continue
				}
				fields := strings.Fields(text)
				if len(fields) < 3 || fields[0] != "omalint:ignore" {
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
				text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
				if i := strings.Index(text, "// want"); i >= 0 {
					text = strings.TrimSpace(text[:i])
				}
				if !strings.HasPrefix(text, "omalint:ignore") {
					continue
				}
				fields := strings.Fields(text)
				if len(fields) < 3 || fields[0] != "omalint:ignore" {
					continue
				}
				if fields[1] == rule && comment.End() <= pos && pass.Fset.Position(pos).Line-pass.Fset.Position(comment.Pos()).Line <= 1 {
					ignored = true
				}
			}
		}
	}
	return ignored
}
