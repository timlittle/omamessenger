package sandbox_test

// helpers_test.go holds what every black-box test in this package
// shares: skipping the real sandbox pipeline's own tests on a machine
// that cannot actually run it (no bwrap or prlimit, or unprivileged
// namespaces disabled), the only reason any of them should ever fail
// here rather than reporting what they found.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

// requireSandbox returns a Runner backed by t.TempDir(), skipping the
// test unless it can actually run binary end to end right now.
func requireSandbox(t *testing.T, binary string) *sandbox.Runner {
	t.Helper()

	r := sandbox.NewRunner(t.TempDir())
	if err := r.Available(t.Context(), binary); err != nil {
		t.Skipf("sandbox unavailable on this machine: %v", err)
	}

	return r
}

// mustOutputFile creates an empty file inside a fresh temporary
// directory and returns its path: sandbox.Spec.Output must already
// exist before Run can bind it, the same way a real caller - such as
// transcode's own joboutput.go - always creates it first.
func mustOutputFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("creating fixture output file: %v", err)
	}

	return path
}
