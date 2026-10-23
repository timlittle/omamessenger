// attachment_retry_recopy_test.go covers recopyFromOriginal's own
// guards, beyond the "file gone" and "file changed" cases
// attachment_test.go already covers for a disk-attached file - split
// out to keep that file within the project's own line-length
// guideline.
package app_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestRetry_FailsClearlyWhenAReusedAttachmentsOutgoingCopyIsMissing
// covers recopyFromOriginal's own guard for media with no OriginalPath
// at all - exactly what reuseAttachment leaves behind, since the file
// was already copied and sanitized once by media.attach or
// media.paste, with nothing further back to recopy from if this
// outgoing copy ever goes missing (see reuseAttachment's own doc
// comment). This never falls back to TestRetry_FailsClearlyWhenNeitherCopyIsAvailable's
// "file changed" or "file gone" checks at all: there is no original
// file to check in the first place.
func TestRetry_FailsClearlyWhenAReusedAttachmentsOutgoingCopyIsMissing(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	src, err := os.Open(writeTestPNG(t, "src.png", 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	reused, err := f.outgoing.Store(ctx, "m_pending", "photo.png", src)
	if closeErr := src.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	f.dispatcher.err = errors.New("offline")
	failed, err := f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: reused})
	if err != nil || failed.Status != domain.StatusFailed {
		t.Fatalf("Send() = %+v, %v", failed, err)
	}

	if err := f.outgoing.Remove(ctx, failed.ID, failed.Media.FileName); err != nil {
		t.Fatal(err)
	}

	f.dispatcher.err = nil
	if _, err := f.commands.Retry(ctx, failed.ID); !errors.Is(err, app.ErrInvalidInput) ||
		!strings.Contains(err.Error(), "attach it again") {
		t.Fatalf("Retry() error = %v, want an invalid-input error about attaching it again", err)
	}
}

// TestRetry_FailsClearlyWhenTheOriginalCannotBeReopened covers
// recopyFromOriginal's own fallback when the original file still
// exists and still looks unchanged by size and modification time, but
// storeAttachment's copy fails anyway - here, because every permission
// bit was cleared after it was attached, so the file can be stat'd but
// not read.
func TestRetry_FailsClearlyWhenTheOriginalCannotBeReopened(t *testing.T) {
	t.Parallel()
	skipAsRoot(t)

	f, ctx, path, failed := sentFailedAttachment(t)

	if err := f.outgoing.Remove(ctx, failed.ID, failed.Media.FileName); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) }) // let t.TempDir() clean it up

	f.dispatcher.err = nil
	if _, err := f.commands.Retry(ctx, failed.ID); !errors.Is(err, app.ErrInvalidInput) ||
		!strings.Contains(err.Error(), "attach it again") {
		t.Fatalf("Retry() error = %v, want an invalid-input error about attaching it again", err)
	}
}
