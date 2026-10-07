package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestFetchMedia_DownloadsOnceIntoTheCache(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	photo := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	file := domain.Message{
		ID: "m2", ConversationID: chat.ID, RemoteID: "41", Text: "[File]", Created: 2,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "../notes.pdf"},
	}
	for _, m := range []domain.Message{photo, file} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	path, err := f.commands.FetchMedia(ctx, "m1")
	if err != nil || filepath.Base(path) != "m1.jpg" {
		t.Fatalf("FetchMedia = %q, %v; want m1.jpg in the cache", path, err)
	}

	if got, _ := os.ReadFile(path); string(got) != "40" {
		t.Errorf("cached %q, want the downloaded media", got)
	}

	if _, err := f.commands.FetchMedia(ctx, "m1"); err != nil || !slices.Equal(f.media.fetched, []string{"40"}) {
		t.Errorf("second FetchMedia = %v, downloads %v; want the cached copy", err, f.media.fetched)
	}

	if path, err := f.commands.FetchMedia(ctx, "m2"); err != nil || filepath.Base(path) != "m2-notes.pdf" {
		t.Errorf("FetchMedia(file) = %q, %v; want its own name kept", path, err)
	}
}

func TestFetchMedia_Rejects(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	for _, m := range []domain.Message{
		{ID: "plain", ConversationID: chat.ID, RemoteID: "1", Text: "hi", Created: 1},
		{ID: "link", ConversationID: chat.ID, RemoteID: "2", Text: "x.io", Created: 2, Media: &domain.Media{Kind: domain.MediaLink}},
	} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for id, want := range map[string]error{
		" ":       app.ErrInvalidInput,
		"plain":   app.ErrInvalidInput,
		"link":    app.ErrInvalidInput,
		"missing": domain.ErrNotFound,
	} {
		if _, err := f.commands.FetchMedia(ctx, id); !errors.Is(err, want) {
			t.Errorf("FetchMedia(%q) = %v, want %v", id, err, want)
		}
	}
}

func TestFetchMedia_ReportsADownloadFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]",
		Created: 1, Media: &domain.Media{Kind: domain.MediaPhoto},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.err = errors.New("offline")
	if _, err := f.commands.FetchMedia(ctx, "m1"); !errors.Is(err, f.media.err) {
		t.Errorf("FetchMedia = %v, want the download's error", err)
	}
}
