package server_test

import (
	"bytes"
	"image"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/server"
)

// TestMediaCopy_WritesTheFetchedFileToTheClipboard confirms media.copy
// downloads a message's media through the same path media.fetch uses,
// then puts that photo's display copy on the clipboard: re-encoded by
// app.Commands.displayCopy, so its bytes differ from the original
// fetched photo even though its pixels do not (see
// docs/decisions.md).
func TestMediaCopy_WritesTheFetchedFileToTheClipboard(t *testing.T) {
	t.Parallel()

	// A genuine photo: app.CopyMedia now refuses to copy anything whose
	// sniffed bytes do not match the message's own kind (see
	// docs/decisions.md), so this test's own fixture must be a real,
	// decodable image rather than an arbitrary placeholder.
	photo := pngBytes(t, 2, 2)
	s := connectWithMedia(t, fakeMediaWriter{content: photo}, cache.New(t.TempDir(), 1<<20))
	msg := domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "r-1", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	if _, _, err := s.store.AddMessage(t.Context(), msg); err != nil {
		t.Fatal(err)
	}

	if _, err := call[struct{}](t, s, "media.copy", map[string]any{"messageId": "m1"}); err != nil {
		t.Fatalf("media.copy = %v", err)
	}

	if bytes.Equal(s.clipboard.written, photo) {
		t.Error("clipboard holds the original bytes unchanged, want the display copy's own re-encode")
	}

	wantImg, _, err := image.Decode(bytes.NewReader(photo))
	if err != nil {
		t.Fatalf("decode want: %v", err)
	}
	gotImg, _, err := image.Decode(bytes.NewReader(s.clipboard.written))
	if err != nil {
		t.Fatalf("decode clipboard content: %v", err)
	}
	if wantImg.Bounds() != gotImg.Bounds() {
		t.Errorf("clipboard image bounds = %v, want %v", gotImg.Bounds(), wantImg.Bounds())
	}
}

// TestMediaCopy_ReportsTheSafeReasonInItsErrorData confirms a failed
// media.copy answers the same way a failed media.fetch does: the
// fixed "internal error" message, with its safe reason category in
// the error's data field.
func TestMediaCopy_ReportsTheSafeReasonInItsErrorData(t *testing.T) {
	t.Parallel()

	s := connectWithMedia(t, fakeMediaFetcher{err: domain.ErrMediaExpired}, cache.New(t.TempDir(), 1<<20))
	msg := domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "r-1", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo},
	}
	if _, _, err := s.store.AddMessage(t.Context(), msg); err != nil {
		t.Fatal(err)
	}

	_, err := call[any](t, s, "media.copy", map[string]any{"messageId": "m1"})
	if code(err) != server.CodeInternal {
		t.Fatalf("media.copy = %v, want internal error", err)
	}

	data, ok := errorData[struct {
		Reason string `json:"reason"`
	}](err)
	if !ok || data.Reason != "expired" {
		t.Errorf("error data = %+v, ok=%v, want reason \"expired\"", data, ok)
	}
}
