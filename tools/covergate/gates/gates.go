// Package gates is the single source of the C9 per-package statement
// coverage thresholds. covergate enforces them; docscheck verifies that the
// C9 contract in docs/TASKS.md lists the same values.
package gates

import "strings"

// Gate is a minimum statement coverage for one package directory, relative
// to the module root.
type Gate struct {
	Package string
	Min     float64
}

// Gates lists every package with its own threshold.
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

// ToolsPrefix packages share one gate, ToolsMin.
const ToolsPrefix = "tools/"

// ToolsMin is the gate for every package under ToolsPrefix.
const ToolsMin = 90

// For returns the threshold for pkg and whether one is defined.
func For(pkg string) (float64, bool) {
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
