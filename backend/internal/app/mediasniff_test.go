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
// "<id>.file.pdf", not "<id>-invoice.pdf.exe".
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
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || filepath.Base(path) != "m1.file.pdf" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.file.pdf", path, err)
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
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || filepath.Base(path) != "m1.file.bin" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.file.bin", path, err)
	}
}

// TestFetchMedia_RecognisesSVGContentEvenClaimedAsSomethingElse
// confirms the SVG heuristic runs for this naming choice, not only for
// MediaRisk's own classification (see mediarisk_test.go): an SVG image
// is cached as "<id>.file.svg", never the generic "<id>.file.xml"
// every other XML document gets.
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
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || filepath.Base(path) != "m1.file.svg" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.file.svg", path, err)
	}
}

// TestFetchMedia_RecognisesABareSVGWithNoXMLProlog confirms the SVG
// heuristic no longer depends on Go's own sniffer having already
// called the content XML in the first place: a real SVG almost never
// opens with an "<?xml" line, unlike TestFetchMedia_RecognisesSVGContentEvenClaimedAsSomethingElse's
// own fixture, which is the one shape that reaches here already
// labelled "text/xml".
func TestFetchMedia_RecognisesABareSVGWithNoXMLProlog(t *testing.T) {
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

	f.media.data = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || filepath.Base(path) != "m1.file.svg" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.file.svg", path, err)
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
	path, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || filepath.Base(path) != "m1.voice.ogg" {
		t.Fatalf("FetchMedia() = %q, %v; want m1.voice.ogg", path, err)
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
	first, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil {
		t.Fatal(err)
	}

	second, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || second != first || len(f.media.fetched) != 1 {
		t.Errorf("second FetchMedia() = %q, %v, fetched %v; want the cached file reused", second, err, f.media.fetched)
	}
}
