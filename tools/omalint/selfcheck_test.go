package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStandaloneRunOnRealPackage builds omalint and runs it the way `make lint`
// does, against a real backend package. Analyzer unit tests use synthetic
// testdata and cannot notice when golang.org/x/tools stops understanding the
// installed Go toolchain's export data; this test fails in that case.
func TestStandaloneRunOnRealPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the linter binary")
	}
	binary := filepath.Join(t.TempDir(), "omalint")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build omalint: %v\n%s", err, out)
	}
	run := exec.Command(binary, "./backend/internal/domain")
	run.Dir = filepath.Join("..", "..")
	out, err := run.CombinedOutput()
	if strings.Contains(string(out), "internal error") {
		t.Fatalf("omalint could not load packages with this toolchain:\n%s", out)
	}
	if err != nil {
		t.Fatalf("omalint reported findings or failed on a clean package: %v\n%s", err, out)
	}
}
