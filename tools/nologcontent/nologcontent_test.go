package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestNoLogContent(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "github.com/timlittle/omamessenger/backend/internal/store")
}

// TestStandaloneRunOnRealPackages builds the command and runs it the way
// `make lint` does, over the real backend. The fixture test cannot notice
// when golang.org/x/tools stops understanding the installed toolchain's
// export data; this test fails in that case.
func TestStandaloneRunOnRealPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the checker")
	}
	binary := filepath.Join(t.TempDir(), "nologcontent")
	if out, err := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	run := exec.Command(binary, "./backend/...")
	run.Dir = filepath.Join("..", "..")
	out, err := run.CombinedOutput()
	if strings.Contains(string(out), "internal error") {
		t.Fatalf("could not load packages with this toolchain:\n%s", out)
	}
	if err != nil {
		t.Fatalf("findings or failure on the backend: %v\n%s", err, out)
	}
}
