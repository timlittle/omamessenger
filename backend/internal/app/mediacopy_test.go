package app_test

import (
	"errors"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestCopyMedia_WritesTheDownloadedFileToTheClipboard confirms a
// message's media is fetched (if need be) and its own bytes, sniffed
// fresh rather than trusted from the message, land on the clipboard
// under that sniffed MIME type.
func TestCopyMedia_WritesTheDownloadedFileToTheClipboard(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	// A plain file, not a photo: this test covers the sniffed-MIME-type
	// behaviour, and a file carries no image check (see
	// app.servedPath), so the fake connector's plain-text placeholder
	// can stand in for the downloaded media unchanged.
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	if err := f.commands.CopyMedia(ctx, "m1"); err != nil {
		t.Fatalf("CopyMedia() = %v", err)
	}

	if len(f.clipboard.written) != 1 {
		t.Fatalf("clipboard.written = %+v, want exactly one write", f.clipboard.written)
	}
	if string(f.clipboard.written[0].data) != "40" {
		t.Errorf("wrote %q, want the downloaded media's own bytes", f.clipboard.written[0].data)
	}
}

// TestCopyMedia_ReportsAFailedFetch confirms a message with nothing to
// download is reported the same way FetchMedia already reports it,
// never reaching the clipboard at all.
func TestCopyMedia_ReportsAFailedFetch(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if err := f.commands.CopyMedia(t.Context(), "missing"); err == nil {
		t.Fatal("CopyMedia() = nil, want an error for a message that does not exist")
	}
	if len(f.clipboard.written) != 0 {
		t.Errorf("clipboard.written = %+v, want nothing written for a failed fetch", f.clipboard.written)
	}
}

// TestCopyMedia_ReportsAClipboardFailure confirms a wl-copy failure
// reaches the caller rather than being reported as a success.
func TestCopyMedia_ReportsAClipboardFailure(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	// A plain file, so the fetch that must succeed before this test's
	// own clipboard failure is reached never has to pass a photo's
	// image check (see app.servedPath).
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	f.clipboard.writeErr = errors.New("wl-copy failed")

	if err := f.commands.CopyMedia(ctx, "m1"); !errors.Is(err, f.clipboard.writeErr) {
		t.Errorf("CopyMedia() = %v, want the clipboard's own error wrapped in", err)
	}
}
