package docscheck

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/tools/covergate/gates"
	"github.com/timlittle/omamessenger/tools/docscheck/markdown"
	"github.com/timlittle/omamessenger/tools/omalint/rules"
)

const root = "../.."

// inventory is the documentation reviewed at every gate (§0.1 D11).
var inventory = []string{
	"README.md", "CONTRIBUTING.md", "AGENTS.md", "FORGE_SPEC.md", ".agents/README.md",
	"docs/TASKS.md", "docs/KEYS.md", "docs/PROTOCOL.md", "docs/ARCHITECTURE.md",
}

// plannedDocs name files that do not exist yet; they are exempt from the
// path, make-target and flag checks but their links must still resolve.
const plannedDocs = "docs/TASKS.md"

type doc struct{ name, text string }

func docs(t *testing.T) []doc {
	t.Helper()
	var out []doc
	for _, name := range inventory {
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, doc{name, string(data)})
	}
	return out
}

func read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestContractC10MatchesRules(t *testing.T) {
	table := markdown.LayeringTable(markdown.Section(read(t, "docs/TASKS.md"), "### C10"), rules.InternalImportPrefix)
	if !reflect.DeepEqual(table, rules.AllowedImports) {
		t.Errorf("C10 layering table and tools/omalint/rules disagree:\nC10:   %v\nrules: %v", table, rules.AllowedImports)
	}
}

func TestContractC9MatchesCovergate(t *testing.T) {
	documented := markdown.CoverageGates(markdown.Section(read(t, "docs/TASKS.md"), "### C9"))
	code := map[string]float64{"tools/*": gates.ToolsMin}
	for _, gate := range gates.Gates {
		code[gate.Package] = gate.Min
	}
	if !reflect.DeepEqual(documented, code) {
		t.Errorf("C9 gates and tools/covergate/gates disagree:\nC9:   %v\ncode: %v", documented, code)
	}
}

func TestLinksResolve(t *testing.T) {
	for _, d := range docs(t) {
		for _, link := range markdown.RelativeLinks(d.text) {
			if _, err := os.Stat(filepath.Join(root, filepath.Dir(d.name), link)); err != nil {
				t.Errorf("%s links to %s, which does not exist", d.name, link)
			}
		}
	}
}

func TestPathsExist(t *testing.T) {
	for _, d := range docs(t) {
		if d.name == plannedDocs {
			continue
		}
		for _, ref := range markdown.RepoPaths(d.text) {
			if !ref.Planned && !markdown.PathExists(root, ref.Path) {
				t.Errorf("%s names %s, which does not exist (label the line \"(planned)\" if it is future work)", d.name, ref.Path)
			}
		}
	}
}

func TestMakeTargetsExist(t *testing.T) {
	targets := markdown.MakefileTargets(read(t, "Makefile"))
	for _, d := range docs(t) {
		if d.name == plannedDocs {
			continue
		}
		for _, target := range markdown.MakeTargets(d.text) {
			if !targets[target] {
				t.Errorf("%s mentions `make %s`, which the Makefile does not define", d.name, target)
			}
		}
	}
}

func TestFlagsExist(t *testing.T) {
	defined, err := markdown.DefinedFlags([]byte(read(t, "backend/config.go")))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "CONTRIBUTING.md"} {
		for _, flag := range markdown.HelperFlags(read(t, name)) {
			if !defined[flag] {
				t.Errorf("%s mentions --%s, which the helper does not define", name, flag)
			}
		}
	}
}

func TestKeysBlockCurrent(t *testing.T) {
	block, ok := markdown.MarkedBlock(read(t, "README.md"), "keys")
	if !ok {
		t.Skip("README has no keys block yet (B15)")
	}
	if keys := read(t, "docs/KEYS.md"); !strings.Contains(keys, block) {
		t.Error("README keys block differs from docs/KEYS.md; run node tools/uilint/gen-keys.cjs")
	}
}
