// attachment_internal_test.go tests sanitizeAttachedPhoto directly,
// an internal test rather than a black-box one because the generic
// "could not be read" branch it covers needs a filesystem failure
// Send's own public API has no way to engineer: storeAttachment
// always leaves a real, freshly written file in place immediately
// before describeAttachment reads it, so the only practical way to
// reach connector.SanitizeOutgoingImageFile's catch-all error path is
// to hand sanitizeAttachedPhoto something that is not a plain file at
// all.
package app

import "testing"

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
