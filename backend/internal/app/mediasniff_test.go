package app_test

import (
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestFetchMedia_NamesAFileBySniffedContentNeverTheSendersClaim
// reproduces the review finding this fixes: a sender names a file's
// media however they like, but the cache must name it from what the
// downloaded bytes actually are, never that claim. A genuine PDF kept
// under a name claiming to be an executable is still cached as
// "<id>.pdf", not "<id>-invoice.pdf.exe".
func TestFetchMedia_NamesAFileBySniffedContentNeverTheSendersClaim(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "invoice.pdf.exe"},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte("%PDF-1.4\nreal pdf bytes, whatever the sender's own file name claims\n")
	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.pdf" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.pdf", path, err)
	}
}

// TestFetchMedia_FallsBackToBinForContentItDoesNotRecognise confirms
// nothing is ever assumed safe just because it was not sniffed as
// anything on the allow-list.
func TestFetchMedia_FallsBackToBinForContentItDoesNotRecognise(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "setup.exe"},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00") // a Windows executable header
	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.bin" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.bin", path, err)
	}
}

// TestFetchMedia_RecognisesSVGContentEvenClaimedAsSomethingElse
// confirms the SVG heuristic runs for this naming choice, not only for
// MediaRisk's own classification (see mediarisk_test.go): an SVG image
// is cached as "<id>.svg", never the generic "<id>.xml" every other
// XML document gets.
func TestFetchMedia_RecognisesSVGContentEvenClaimedAsSomethingElse(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "photo.jpg"},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.svg" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.svg", path, err)
	}
}

// TestFetchMedia_VoiceNoteAlsoNamesBySniffedContent confirms the fix
// applies to every kind but a photo or video, not only MediaFile.
func TestFetchMedia_VoiceNoteAlsoNamesBySniffedContent(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Voice message]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg"},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte("OggS\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00")
	path, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || filepath.Base(path) != "m1.ogg" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.ogg", path, err)
	}
}

// TestFetchMedia_SniffedNameIsCachedAndNeverReDownloaded confirms
// FetchNamed's own lookup finds the file a first fetch sniffed and
// named, the same way the old, fixed naming already did, so a second
// fetch for the same message never re-downloads it.
func TestFetchMedia_SniffedNameIsCachedAndNeverReDownloaded(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "report.pdf"},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = []byte("%PDF-1.4\n")
	first, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil {
		t.Fatal(err)
	}

	second, err := f.commands.FetchMedia(ctx, "m1", false)
	if err != nil || second != first || len(f.media.fetched) != 1 {
		t.Errorf("second FetchMedia() = %q, %v, fetched %v; want the cached file reused", second, err, f.media.fetched)
	}
}
