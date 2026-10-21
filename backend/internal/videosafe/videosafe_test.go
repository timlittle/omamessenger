package videosafe_test

// videosafe_test.go checks Validate's own top-level dispatch: a file
// too short to even hold a container magic, and a file that cannot be
// opened at all. Dispatch to each container's own walker is already
// covered by isobmff_test.go and matroska_test.go's own fixtures.

import (
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/videosafe"
)

// TestValidate_TooShortToHaveAContainerMagic covers a file with fewer
// than the dozen bytes any format this package recognizes needs just
// to name itself.
func TestValidate_TooShortToHaveAContainerMagic(t *testing.T) {
	t.Parallel()

	path := writeFixture(t, "short.bin", []byte{0, 1, 2, 3})
	got, err := videosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true for a file too short to be any recognized container")
	}
}

// TestValidate_MissingFile covers the one case Validate reports as an
// actual error, rather than simply "not playable": a file that
// cannot be opened at all.
func TestValidate_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := videosafe.Validate(filepath.Join(t.TempDir(), "missing.bin"))
	if err == nil {
		t.Fatal("Validate() on a missing file succeeded")
	}
}
