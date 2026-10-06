package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/tools/docscheck/markdown"
)

const root = "../.."

// inventory is the documentation reviewed at every gate (§0.1 D11).
var inventory = []string{
	"README.md", "CONTRIBUTING.md", "AGENTS.md", "FORGE_SPEC.md", ".agents/README.md",
	"docs/TASKS.md", "docs/KEYS.md", "docs/PROTOCOL.md",
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
	buildOutput := markdown.IgnoredDirs(read(t, ".gitignore"))
	for _, d := range docs(t) {
		if d.name == plannedDocs {
			continue
		}
		for _, ref := range markdown.RepoPaths(d.text) {
			if !ref.Planned && !underAny(ref.Path, buildOutput) && !markdown.PathExists(root, ref.Path) {
				t.Errorf("%s names %s, which does not exist (label the line \"(planned)\" if it is future work)", d.name, ref.Path)
			}
		}
	}
}

// underAny reports whether p is inside one of dirs (build output that a
// clean checkout does not contain).
func underAny(p string, dirs []string) bool {
	for _, dir := range dirs {
		if strings.HasPrefix(p, dir) {
			return true
		}
	}
	return false
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
