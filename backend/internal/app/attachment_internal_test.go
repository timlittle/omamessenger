// attachment_internal_test.go tests sanitizeAttachedPhoto, photoThumb,
// imageDimensions and sniffKind directly, an internal test rather than
// a black-box one because the one branch each has left to cover - the
// file disappearing between Send's own os.Stat and the moment one of
// these helpers opens it - needs a filesystem failure Send's own
// public API has no way to engineer: every path these functions ever
// see from Send or AttachFile was just written or renamed into place
// immediately before, so it is always there. A path that simply never
// existed reaches the same branch just as well, with no race to set up.
package app

import (
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSanitizeAttachedPhoto_GenericReadFailure covers the one branch
// neither "too large" nor "unsafe" names: anything else
// SanitizeOutgoingImageFile can fail with, such as path naming a
// directory instead of a file.
func TestSanitizeAttachedPhoto_GenericReadFailure(t *testing.T) {
	t.Parallel()

	if err := sanitizeAttachedPhoto(t.TempDir()); err == nil {
		t.Fatal("sanitizeAttachedPhoto(directory) succeeded, want an error")
	}
}

// TestPhotoThumb_EmptyWhenTheFileCannotBeOpened covers photoThumb's own
// read failure, never reached through Send or AttachFile: it only ever
// runs on a path those just finished writing.
func TestPhotoThumb_EmptyWhenTheFileCannotBeOpened(t *testing.T) {
	t.Parallel()

	if got := photoThumb(filepath.Join(t.TempDir(), "missing.png")); got != "" {
		t.Errorf("photoThumb(missing file) = %q, want empty", got)
	}
}

// TestImageDimensions_ZeroWhenTheFileCannotBeOpened mirrors
// TestPhotoThumb_EmptyWhenTheFileCannotBeOpened for imageDimensions.
func TestImageDimensions_ZeroWhenTheFileCannotBeOpened(t *testing.T) {
	t.Parallel()

	w, h := imageDimensions(filepath.Join(t.TempDir(), "missing.png"))
	if w != 0 || h != 0 {
		t.Errorf("imageDimensions(missing file) = %d,%d, want 0,0", w, h)
	}
}

// TestSniffKind_FileWhenTheFileCannotBeOpened mirrors the same case for
// sniffKind, which falls back to treating an unreadable path as a plain
// file rather than guessing.
func TestSniffKind_FileWhenTheFileCannotBeOpened(t *testing.T) {
	t.Parallel()

	if got := sniffKind(filepath.Join(t.TempDir(), "missing")); got != domain.MediaFile {
		t.Errorf("sniffKind(missing file) = %q, want %q", got, domain.MediaFile)
	}
}
