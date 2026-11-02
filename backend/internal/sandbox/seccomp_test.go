package sandbox

// seccomp_test.go tests seccompProgram and seccompExtraFile directly,
// as an internal test: both are unexported, and assembling the
// program itself needs no real bwrap, so it is worth proving on its
// own rather than only through the slower, skip-if-unavailable real
// enforcement test in sandbox_test.go (TestRun_SeccompDeniesUnshare).

import (
	"io"
	"testing"
)

func TestSeccompProgram(t *testing.T) {
	t.Parallel()

	program, err := seccompProgram()
	if err != nil {
		t.Fatalf("seccompProgram() error = %v", err)
	}

	if len(program) == 0 {
		t.Fatal("seccompProgram() returned no bytes")
	}
	if len(program)%8 != 0 {
		t.Fatalf("seccompProgram() returned %d bytes, want a multiple of 8 (struct sock_filter is 8 bytes)", len(program))
	}

	again, err := seccompProgram()
	if err != nil {
		t.Fatalf("second seccompProgram() error = %v", err)
	}
	if string(program) != string(again) {
		t.Error("seccompProgram() is not deterministic across calls")
	}
}

func TestSeccompExtraFile(t *testing.T) {
	t.Parallel()

	want, err := seccompProgram()
	if err != nil {
		t.Fatalf("seccompProgram() error = %v", err)
	}

	f, err := seccompExtraFile()
	if err != nil {
		t.Fatalf("seccompExtraFile() error = %v", err)
	}
	defer f.Close()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading seccompExtraFile's own fd: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("seccompExtraFile() holds %d bytes, want the %d bytes seccompProgram() returns", len(got), len(want))
	}
}
