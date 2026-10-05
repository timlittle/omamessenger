// Package rules contains the single source of truth for omalint thresholds.
package rules

const (
	MaxFileLines     = 400
	MaxFunctionLines = 60
	MaxParameters    = 5
)

// InternalImportPrefix is the project module path used to identify internal edges.
const InternalImportPrefix = "github.com/timlittle/omamessenger/backend/internal/"

const (
	ToolsPrefix        = "github.com/timlittle/omamessenger/tools/"
	DocscheckPrefix    = "github.com/timlittle/omamessenger/tools/docscheck"
	DocscheckAPIImport = "github.com/timlittle/omamessenger/backend/internal/api"
)

// AllowedImports lists the internal package imports permitted by C10.
var AllowedImports = map[string]map[string]bool{
	"github.com/timlittle/omamessenger/backend/internal/domain": {},
	"github.com/timlittle/omamessenger/backend/internal/store": {
		"github.com/timlittle/omamessenger/backend/internal/domain": true,
	},
	"github.com/timlittle/omamessenger/backend/internal/connector": {
		"github.com/timlittle/omamessenger/backend/internal/domain": true,
	},
	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest": {
		"github.com/timlittle/omamessenger/backend/internal/domain": true,
	},
	"github.com/timlittle/omamessenger/backend/internal/connector/demo": {
		"github.com/timlittle/omamessenger/backend/internal/connector": true,
		"github.com/timlittle/omamessenger/backend/internal/domain":    true,
	},
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest": {
		"github.com/timlittle/omamessenger/backend/internal/connector":           true,
		"github.com/timlittle/omamessenger/backend/internal/domain":              true,
		"github.com/timlittle/omamessenger/backend/internal/connector/clocktest": true,
	},
	"github.com/timlittle/omamessenger/backend/internal/notify": {},
	"github.com/timlittle/omamessenger/backend/internal/app/policy": {
		"github.com/timlittle/omamessenger/backend/internal/domain": true,
	},
	"github.com/timlittle/omamessenger/backend/internal/app": {
		"github.com/timlittle/omamessenger/backend/internal/domain":     true,
		"github.com/timlittle/omamessenger/backend/internal/connector":  true,
		"github.com/timlittle/omamessenger/backend/internal/app/policy": true,
	},
	"github.com/timlittle/omamessenger/backend/internal/rpc": {},
	"github.com/timlittle/omamessenger/backend/internal/api": {
		"github.com/timlittle/omamessenger/backend/internal/app":    true,
		"github.com/timlittle/omamessenger/backend/internal/rpc":    true,
		"github.com/timlittle/omamessenger/backend/internal/domain": true,
	},
}

// TestOnlyImports are the extra edges allowed from package test files.
var TestOnlyImports = map[string]bool{
	"github.com/timlittle/omamessenger/backend/internal/store":                   true,
	"github.com/timlittle/omamessenger/backend/internal/notify":                  true,
	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest":     true,
	"github.com/timlittle/omamessenger/backend/internal/connector/demo":          true,
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest": true,
}
