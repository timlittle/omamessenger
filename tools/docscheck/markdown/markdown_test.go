package markdown

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

const contracts = "intro\n" +
	"### C3 · Protocol\n" +
	"| method | params |\n|---|---|\n| `hello` | `{}` |\n| `messages.send` | `{text}` |\n\n" +
	"| event | data |\n|---|---|\n| `typing` | `{}` |\n" +
	"### C4 · Next\n| method | x |\n|---|---|\n| `outside` | x |\n"

func TestSectionAndTableNames(t *testing.T) {
	c3 := Section(contracts, "### C3")
	got := TableNames(c3)
	want := map[string][]string{"method": {"hello", "messages.send"}, "event": {"typing"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TableNames = %v, want %v", got, want)
	}
	if Section(contracts, "### C9") != "" {
		t.Error("missing section should be empty")
	}
	if last := Section(contracts, "### C4"); TableNames(last)["method"][0] != "outside" {
		t.Errorf("last section = %q", last)
	}
}

func TestLayeringTable(t *testing.T) {
	md := "| package | may import |\n|---|---|\n" +
		"| `backend/internal/domain` | — |\n" +
		"| `backend/internal/store` | domain (+ `modernc.org/sqlite`) |\n" +
		"| `backend/internal/connector`, `connector/clocktest` | domain |\n" +
		"| `backend/internal/app` | domain, connector, app/policy |\n" +
		"| `backend/internal/archtest` | — (test-only package) |\n" +
		"| `backend` (main) | any internal package |\n" +
		"| `tools/*` | nothing |\n"
	const p = "x/"
	want := map[string]map[string]bool{
		"x/domain":              {},
		"x/store":               {"x/domain": true},
		"x/connector":           {"x/domain": true},
		"x/connector/clocktest": {"x/domain": true},
		"x/app":                 {"x/domain": true, "x/connector": true, "x/app/policy": true},
		"x/archtest":            {},
	}
	if got := LayeringTable(md, p); !reflect.DeepEqual(got, want) {
		t.Errorf("LayeringTable = %v, want %v", got, want)
	}
}

func TestCoverageGates(t *testing.T) {
	md := "- 100 %: `domain`, `app/policy`\n" +
		"- ≥ 90 %: `store`, every `tools/*` package\n" +
		"- ≥ 80 %: `backend` (main); only `func main` is excluded\n" +
		"- not a gate: `ignored`\n"
	want := map[string]float64{
		"backend/internal/domain": 100, "backend/internal/app/policy": 100,
		"backend/internal/store": 90, "tools/*": 90, "backend": 80,
	}
	if got := CoverageGates(md); !reflect.DeepEqual(got, want) {
		t.Errorf("CoverageGates = %v, want %v", got, want)
	}
}

func TestIgnoredDirs(t *testing.T) {
	got := IgnoredDirs("/messages.db\n/bin/dev/\n*.session\n  /build/  \nnode_modules/\n")
	if !reflect.DeepEqual(got, []string{"bin/dev/", "build/"}) {
		t.Errorf("IgnoredDirs = %v", got)
	}
}

func TestRelativeLinks(t *testing.T) {
	md := "[a](CONTRIBUTING.md) [b](docs/KEYS.md#list) [c](https://example.com) [d](mailto:x@y) [e](#anchor)"
	if got := RelativeLinks(md); !reflect.DeepEqual(got, []string{"CONTRIBUTING.md", "docs/KEYS.md"}) {
		t.Errorf("RelativeLinks = %v", got)
	}
}

func TestRepoPathsAndExistence(t *testing.T) {
	md := "See `backend/main.go`.\nAnd `ui/Panel.qml` (planned).\nNot a path: `README.md`, `ui/`.\n"
	got := RepoPaths(md)
	want := []PathRef{{Path: "backend/main.go"}, {Path: "ui/Panel.qml", Planned: true}, {Path: "ui/"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RepoPaths = %+v, want %+v", got, want)
	}
	root := t.TempDir()
	for _, f := range []string{"backend/main.go", "backend/internal/a/a_test.go", "tests/unit/x.cjs"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for p, want := range map[string]bool{
		"backend/main.go": true, "backend/": true, "backend/internal/*/*_test.go": true,
		"tests/{unit,qml}/": false, "tests/{unit}/x.cjs": true, "ui/Panel.qml": false, "[": false,
	} {
		if got := PathExists(root, p); got != want {
			t.Errorf("PathExists(%q) = %t, want %t", p, got, want)
		}
	}
}

func TestCodeLinesMakeTargetsAndFlags(t *testing.T) {
	md := "Run `make test` but make the window nice.\n" +
		"```sh\nmake install-local\ngo run ./backend --demo --data-dir x\nomarchy plugin add url --enable\ngit pull --ff-only\n```\n" +
		"Use `--no-chatter` or `--seed N`.\n"
	targets := MakeTargets(md)
	sort.Strings(targets)
	if !reflect.DeepEqual(targets, []string{"install-local", "test"}) {
		t.Errorf("MakeTargets = %v", targets)
	}
	flags := HelperFlags(md)
	sort.Strings(flags)
	if !reflect.DeepEqual(flags, []string{"data-dir", "demo", "no-chatter", "seed"}) {
		t.Errorf("HelperFlags = %v (other tools' flags must be skipped)", flags)
	}
	if got := MakefileTargets("all: build\nbuild: ## x\n\tgo build\n.PHONY: x\n"); !got["all"] || !got["build"] || got["go"] {
		t.Errorf("MakefileTargets = %v", got)
	}
}

func TestDefinedFlags(t *testing.T) {
	src := []byte(`package main
import "flag"
func f(fs *flag.FlagSet) {
	var b bool
	var s string
	fs.BoolVar(&b, "demo", false, "")
	fs.StringVar(&s, "data-dir", "", "")
	fs.Int64("seed", 0, "")
	fs.Parse(nil)
	other.BoolVar(&b, nameVariable, false, "")
	fs.Bool()
}`)
	got, err := DefinedFlags(src)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{"demo": true, "data-dir": true, "seed": true}; !reflect.DeepEqual(got, want) {
		t.Errorf("DefinedFlags = %v, want %v", got, want)
	}
	if _, err := DefinedFlags([]byte("not go")); err == nil {
		t.Error("want a parse error")
	}
}

func TestMarkedBlock(t *testing.T) {
	md := "a\n<!-- keys:start -->\n| key |\n<!-- keys:end -->\nb"
	if got, ok := MarkedBlock(md, "keys"); !ok || got != "| key |" {
		t.Errorf("MarkedBlock = %q, %t", got, ok)
	}
	if _, ok := MarkedBlock("<!-- keys:end --> <!-- keys:start -->", "keys"); ok {
		t.Error("reversed markers must not match")
	}
}
