package server_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/server"
)

// TestMediaSave_CopiesTheFetchedFileIntoDownloads confirms media.save
// downloads a message's media through the same path media.fetch uses,
// then leaves a copy of it in the helper's downloads directory.
func TestMediaSave_CopiesTheFetchedFileIntoDownloads(t *testing.T) {
	t.Parallel()

	s, downloads := connectWithMediaSave(t, fakeMediaWriter{content: []byte("a photo")}, cache.New(t.TempDir(), 1<<20))
	msg := domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "r-1", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	if _, _, err := s.store.AddMessage(t.Context(), msg); err != nil {
		t.Fatal(err)
	}

	got, err := call[struct {
		Path string `json:"path"`
	}](t, s, "media.save", map[string]any{"messageId": "m1"})
	if err != nil {
		t.Fatalf("media.save = %v", err)
	}

	if filepath.Dir(got.Path) != downloads {
		t.Errorf("media.save saved to %q, want it under %q", got.Path, downloads)
	}

	if content, err := os.ReadFile(got.Path); err != nil || string(content) != "a photo" {
		t.Errorf("saved file = %q, %v; want the fetched media", content, err)
	}
}

// TestMediaSave_ReportsTheSafeReasonInItsErrorData confirms a failed
// media.save answers the same way a failed media.fetch does: the fixed
// "internal error" message, with its safe reason category in the
// error's data field.
func TestMediaSave_ReportsTheSafeReasonInItsErrorData(t *testing.T) {
	t.Parallel()

	s, _ := connectWithMediaSave(t, fakeMediaFetcher{err: domain.ErrMediaExpired}, cache.New(t.TempDir(), 1<<20))
	msg := domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "r-1", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo},
	}
	if _, _, err := s.store.AddMessage(t.Context(), msg); err != nil {
		t.Fatal(err)
	}

	_, err := call[any](t, s, "media.save", map[string]any{"messageId": "m1"})
	if code(err) != server.CodeInternal {
		t.Fatalf("media.save = %v, want internal error", err)
	}

	data, ok := errorData[struct {
		Reason string `json:"reason"`
	}](err)
	if !ok || data.Reason != "expired" {
		t.Errorf("error data = %+v, ok=%v, want reason \"expired\"", data, ok)
	}
}

// TestMediaSave_RejectsAnUnknownMessage confirms media.save answers
// invalid params for a message id the store has never heard of, the
// same as media.fetch.
func TestMediaSave_RejectsAnUnknownMessage(t *testing.T) {
	t.Parallel()

	s, _ := connectWithMediaSave(t, fakeMediaFetcher{}, cache.New(t.TempDir(), 1<<20))

	if _, err := call[any](t, s, "media.save", map[string]any{"messageId": "missing"}); code(err) != server.CodeNotFound {
		t.Errorf("media.save(missing) = %v, want not found", err)
	}
}
