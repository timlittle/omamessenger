package audiosafe_test

// audiosafe_test.go checks Validate's own dispatch and error
// handling, the behaviour every format-specific test file shares,
// kept here rather than duplicated in each of them.

import (
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
)

// TestValidate_MissingFileIsAnError covers the one case Validate
// reports an error for: the file itself cannot be opened, as opposed
// to every other way its content fails a check, which is reported as
// simply not playable.
func TestValidate_MissingFileIsAnError(t *testing.T) {
	t.Parallel()

	_, err := audiosafe.Validate(filepath.Join(t.TempDir(), "missing.audio"))
	if err == nil {
		t.Fatal("Validate() error = nil, want an error for a missing file")
	}
}

// TestValidate_UnrecognizedFormat covers a file that is neither Ogg,
// MP4/M4A nor a plausible MP3 frame stream: reported false, with no
// error, the same way every other unsafe or unsupported input is.
func TestValidate_UnrecognizedFormat(t *testing.T) {
	t.Parallel()

	path := writeFixture(t, "unknown.bin", []byte("this is not any audio format at all"))
	got, err := audiosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true, want false")
	}
}

// TestValidate_EmptyFile covers a zero-length file, too short for
// any format's own magic bytes.
func TestValidate_EmptyFile(t *testing.T) {
	t.Parallel()

	path := writeFixture(t, "empty.audio", []byte{})
	got, err := audiosafe.Validate(path)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if got {
		t.Error("Validate() = true, want false")
	}
}
