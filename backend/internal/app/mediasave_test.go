package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSaveMedia_CopiesIntoTheDownloadsDirectory(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil {
		t.Fatalf("SaveMedia = %v", err)
	}

	if filepath.Dir(path) != f.downloads {
		t.Errorf("SaveMedia saved to %q, want it under %q", path, f.downloads)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != "40" {
		t.Errorf("saved file = %q, %v; want the downloaded media", got, err)
	}
}

func TestSaveMedia_NeverOverwritesAnEarlierSave(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	first, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil {
		t.Fatalf("first SaveMedia = %v", err)
	}

	second, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil {
		t.Fatalf("second SaveMedia = %v", err)
	}

	if first == second {
		t.Fatalf("second SaveMedia reused %q, want a distinct name", second)
	}

	if _, err := os.Stat(first); err != nil {
		t.Errorf("first save %q no longer exists: %v", first, err)
	}
}

func TestSaveMedia_KeepsTheSenderOwnFileName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "report.pdf"},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil || filepath.Base(path) != "report.pdf" {
		t.Fatalf("SaveMedia = %q, %v; want report.pdf", path, err)
	}
}

func TestSaveMedia_RejectsAnUnknownMessage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.SaveMedia(t.Context(), "missing"); err == nil {
		t.Error("SaveMedia(missing) = nil error, want one")
	}
}

func TestSaveMedia_RejectsWithNoDownloadsDirectoryConfigured(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	// appOver's own Deps set no DownloadsDir, which is exactly the case
	// this test wants: a helper that started with no HOME to resolve
	// one from.
	commands, _, _, _ := appOver(t, f.store)

	_, err := commands.SaveMedia(ctx, "m1")
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SaveMedia with no downloads directory = %v, want %v", err, app.ErrInvalidInput)
	}
}
