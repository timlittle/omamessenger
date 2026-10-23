package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAttachFile_CopiesAndSanitizesAValidImage confirms a photo picked
// from disk is copied into the outgoing media area and run through
// imagesafe, the same as a downloaded or pasted one, before the
// composer is ever told it is safe to preview.
func TestAttachFile_CopiesAndSanitizesAValidImage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := writeTestPNG(t, "vacation.png", 5, 4)

	got, err := f.commands.AttachFile(t.Context(), path)
	if err != nil {
		t.Fatalf("AttachFile() error = %v", err)
	}

	if got.Kind != domain.MediaPhoto || got.Name != "vacation.png" || got.Width != 5 || got.Height != 4 {
		t.Errorf("AttachFile() = %+v, want a 5x4 photo named vacation.png", got)
	}
	if got.Path == path {
		t.Errorf("AttachFile() Path = %q, want a copy, not the original path", got.Path)
	}
	if info, err := os.Stat(got.Path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("stored copy mode = %v, %v, want 0600", info, err)
	}
}

// TestAttachFile_RejectsAnOversizedImage confirms a photo declaring
// more bytes than imagesafe's own FullImage profile accepts is refused
// at attach time, before the composer ever shows a preview of it,
// rather than waiting until the message is actually sent.
func TestAttachFile_RejectsAnOversizedImage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	oversized := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 33*1024*1024)...)
	path := writeTestFile(t, "huge.png", oversized)

	if _, err := f.commands.AttachFile(t.Context(), path); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("AttachFile(oversized image) = %v, want ErrInvalidInput", err)
	}
}

// TestAttachFile_AcceptsANonImageFileAsAPlainFile confirms a file
// whose content is not a photo is still attached, as a plain file with
// no dimensions or thumbnail, never refused just for not being an image.
func TestAttachFile_AcceptsANonImageFileAsAPlainFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := writeTestFile(t, "notes.txt", []byte("plain text file"))

	got, err := f.commands.AttachFile(t.Context(), path)
	if err != nil {
		t.Fatalf("AttachFile() error = %v", err)
	}

	if got.Kind != domain.MediaFile || got.Name != "notes.txt" || got.Width != 0 || got.Height != 0 {
		t.Errorf("AttachFile() = %+v, want a plain file named notes.txt with no dimensions", got)
	}
}

// TestAttachFile_RejectsAMissingFile confirms a path that no longer
// exists - the user picked it, then moved or deleted it before the
// helper got to read it - fails with a safe, invalid-input message.
func TestAttachFile_RejectsAMissingFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	path := filepath.Join(t.TempDir(), "missing")

	if _, err := f.commands.AttachFile(t.Context(), path); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("AttachFile(missing file) = %v, want ErrInvalidInput", err)
	}
}

// TestAttachFile_RejectsAnEmptyPath confirms media.attach always
// requires a path - the UI never calls it with none.
func TestAttachFile_RejectsAnEmptyPath(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.AttachFile(t.Context(), ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("AttachFile(\"\") = %v, want ErrInvalidInput", err)
	}
}

// TestSend_ReusesAnAttachedFileWithoutCopyingOrSanitizingItAgain
// confirms Send, given the path AttachFile already prepared, moves
// that copy into place under the message's own id instead of copying
// it again: AttachFile's own copy is gone afterwards, which a second
// copy-and-sanitize pass (see prepareAttachment's reuseAttachment
// branch) would never do.
func TestSend_ReusesAnAttachedFileWithoutCopyingOrSanitizingItAgain(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	disk := writeTestPNG(t, "vacation.png", 5, 4)

	attached, err := f.commands.AttachFile(ctx, disk)
	if err != nil {
		t.Fatalf("AttachFile() error = %v", err)
	}

	m, err := f.commands.Send(ctx, "chat", "", app.SendOptions{AttachmentPath: attached.Path, AttachmentName: attached.Name})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if m.Media == nil || m.Media.FileName != "vacation.png" || m.Media.Kind != domain.MediaPhoto {
		t.Fatalf("Media = %+v, want the original name and kind kept", m.Media)
	}
	if _, err := os.Stat(attached.Path); !os.IsNotExist(err) {
		t.Errorf("AttachFile's own copy still exists after Send, want it moved rather than copied again (err = %v)", err)
	}
	if _, err := os.Stat(m.Media.Path); err != nil {
		t.Errorf("stored copy: %v", err)
	}

	// The original disk file AttachFile itself copied from is left
	// alone throughout - only AttachFile's own intermediate copy moves.
	if _, err := os.Stat(disk); err != nil {
		t.Errorf("original disk file: %v, want it untouched", err)
	}
}
