package server_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/server"
)

// TestMediaCopy_WritesTheFetchedFileToTheClipboard confirms media.copy
// downloads a message's media through the same path media.fetch uses,
// then puts its own bytes on the clipboard.
func TestMediaCopy_WritesTheFetchedFileToTheClipboard(t *testing.T) {
	t.Parallel()

	s := connectWithMedia(t, fakeMediaWriter{content: []byte("a photo")}, cache.New(t.TempDir(), 1<<20))
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

	if string(s.clipboard.written) != "a photo" {
		t.Errorf("clipboard got %q, want the fetched media", s.clipboard.written)
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
