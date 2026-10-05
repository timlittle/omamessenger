package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mainSrc = `package main

func main() {
	run()
}

func run() {}
`

func profile(lines ...string) string {
	return "mode: atomic\n" + strings.Join(lines, "\n") + "\n"
}

func TestParseProfileAggregatesPerPackage(t *testing.T) {
	in := profile(
		ModulePath+"backend/internal/domain/domain.go:1.1,2.2 3 1",
		ModulePath+"backend/internal/domain/domain.go:3.1,4.2 1 0",
		ModulePath+"backend/internal/store/store.go:1.1,2.2 2 5",
	)
	counts, err := ParseProfile(strings.NewReader(in), lineRange{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := counts["backend/internal/domain"]; got != (Count{4, 3}) {
		t.Fatalf("domain = %+v", got)
	}
	if got := counts["backend/internal/store"]; got != (Count{2, 2}) {
		t.Fatalf("store = %+v", got)
	}
}

func TestParseProfileDeduplicatesBlocks(t *testing.T) {
	block := ModulePath + "backend/internal/app/app.go:10.1,12.2 4 "
	for _, order := range [][]string{{"0", "7"}, {"7", "0"}} {
		in := profile(block+order[0], block+order[1])
		counts, err := ParseProfile(strings.NewReader(in), lineRange{1, 0})
		if err != nil {
			t.Fatal(err)
		}
		if got := counts["backend/internal/app"]; got != (Count{4, 4}) {
			t.Fatalf("order %v: app = %+v", order, got)
		}
	}
}

func TestParseProfileExcludesOnlyMain(t *testing.T) {
	exclude, err := MainRange([]byte(mainSrc))
	if err != nil {
		t.Fatal(err)
	}
	if exclude != (lineRange{3, 5}) {
		t.Fatalf("main range = %+v", exclude)
	}
	in := profile(
		ModulePath+"backend/main.go:3.13,5.2 1 0",
		ModulePath+"backend/main.go:7.12,7.14 1 1",
		ModulePath+"backend/internal/x/main.go:3.13,5.2 1 0",
	)
	counts, err := ParseProfile(strings.NewReader(in), exclude)
	if err != nil {
		t.Fatal(err)
	}
	if got := counts["backend"]; got != (Count{1, 1}) {
		t.Fatalf("backend = %+v (func main must be excluded, run kept)", got)
	}
	if got := counts["backend/internal/x"]; got != (Count{1, 0}) {
		t.Fatalf("only backend/main.go is excluded, got %+v", got)
	}
}

func TestMainRangeWithoutMain(t *testing.T) {
	r, err := MainRange([]byte("package main\n\nfunc run() {}\n"))
	if err != nil || r.start <= r.end {
		t.Fatalf("range = %+v, %v; want empty", r, err)
	}
	if _, err := MainRange([]byte("not go")); err == nil {
		t.Fatal("want parse error")
	}
}

func TestParseProfileRejectsMalformedLines(t *testing.T) {
	for _, bad := range []string{
		"no-colon-here",
		ModulePath + "a.go:1.1,2.2 1",
		ModulePath + "a.go:1.1-2.2 1 1",
		ModulePath + "a.go:x.1,2.2 1 1",
		ModulePath + "a.go:1.1,2.2 one 1",
	} {
		if _, err := ParseProfile(strings.NewReader(profile(bad)), lineRange{1, 0}); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestEvaluate(t *testing.T) {
	results := Evaluate(map[string]Count{
		"backend/internal/domain":   {10, 10},
		"backend/internal/notify":   {100, 74},
		"backend/internal/mystery":  {1, 1},
		"tools/omalint/size":        {10, 9},
		"tools/covergate":           {10, 8},
		"backend/internal/archtest": {0, 0},
	})
	want := map[string]string{
		"backend/internal/domain":  "",
		"backend/internal/notify":  "below gate",
		"backend/internal/mystery": "no gate defined",
		"tools/omalint/size":       "",
		"tools/covergate":          "below gate",
	}
	if len(results) != len(want) {
		t.Fatalf("results = %+v (zero-statement packages must be skipped)", results)
	}
	for _, r := range results {
		if r.Problem != want[r.Package] {
			t.Errorf("%s: problem %q, want %q", r.Package, r.Problem, want[r.Package])
		}
	}
}

func TestRender(t *testing.T) {
	var out bytes.Buffer
	ok := Render(&out, []Result{{Package: "a", Count: Count{2, 1}, Min: 90, Problem: "below gate"}})
	if ok || !strings.Contains(out.String(), "FAIL: below gate") || !strings.Contains(out.String(), "50.0%") {
		t.Fatalf("ok=%v\n%s", ok, out.String())
	}
	out.Reset()
	if !Render(&out, []Result{{Package: "a", Count: Count{1, 1}, Min: 90}}) {
		t.Fatal("passing results must render ok")
	}
}

func TestRunExitCodes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, MainFile), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) string {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pass := write("pass.out", profile(ModulePath+"backend/internal/domain/d.go:1.1,2.2 1 1"))
	fail := write("fail.out", profile(ModulePath+"backend/internal/domain/d.go:1.1,2.2 1 0"))
	bad := write("bad.out", profile("garbage"))
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"-root", root, pass}, 0},
		{[]string{"-root", root, fail}, 1},
		{[]string{"-root", root, bad}, 2},
		{[]string{"-root", root, filepath.Join(root, "missing.out")}, 2},
		{[]string{"-root", filepath.Join(root, "nowhere"), pass}, 0},
		{[]string{}, 2},
		{[]string{"-bogus"}, 2},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if got := run(c.args, &stdout, &stderr); got != c.want {
			t.Errorf("run(%v) = %d, want %d\nstdout:%s\nstderr:%s", c.args, got, c.want, stdout.String(), stderr.String())
		}
	}
}

func TestRunRejectsUnparsableMain(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, MainFile), []byte("not go"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if got := run([]string{"-root", root, "unused.out"}, &stdout, &stderr); got != 2 {
		t.Fatalf("exit = %d, want 2", got)
	}
}
