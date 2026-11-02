package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSaveMedia_CopiesIntoTheDownloadsDirectory(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	// A plain file, not a photo: SaveMedia's own filesystem behaviour is
	// what this test covers, and a file carries no image check (see
	// app.servedPath), so the fake connector's plain-text placeholder
	// can stand in for the downloaded media unchanged.
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
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
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
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

// TestSaveMedia_KeepsTheSenderStemButSniffsTheExtension confirms a
// saved download keeps the sender's own name as its stem, but always
// takes its extension from the downloaded bytes' own sniffed content
// type (see extensionFor), never the sender's own claim: this
// fixture's fake connector always writes the message's remote id as
// plain text, so a sender-claimed ".pdf" must still come back ".txt".
func TestSaveMedia_KeepsTheSenderStemButSniffsTheExtension(t *testing.T) {
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
	if err != nil || filepath.Base(path) != "report.txt" {
		t.Fatalf("SaveMedia = %q, %v; want report.txt", path, err)
	}
}

// TestSaveMedia_StripsControlAndBidiCharactersFromTheFileName confirms
// a sender's claimed file name is sanitized before it is used for a
// saved download: control characters and Unicode bidi/format
// characters (which a sender could use to make a file's real
// extension read differently than it is) are stripped, and runs of
// whitespace collapse to one space.
func TestSaveMedia_StripsControlAndBidiCharactersFromTheFileName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "evil\x00name\u202e  .png"},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil || filepath.Base(path) != "evilname.txt" {
		t.Fatalf("SaveMedia = %q, %v; want evilname.txt", path, err)
	}
}

// TestSaveMedia_NeverWritesMarkupCharactersInTheFileName confirms the
// gap a security review found: a sanitized file name kept markup
// characters - `<`, `>`, `&`, `=`, `#`, `;` - that Qt's non-native
// fallback file dialog (its FileDialogDelegateLabel.qml, used by the
// composer's attach picker and the sound picker) renders as rich text
// with no textFormat override, defaulting to AutoText. A sender's own
// file name containing `<img src=x>` would then make the shell fetch
// a remote image just from listing the downloads folder. None of
// those characters, or any other outside a conservative allow-list,
// must ever reach the saved file's own name.
func TestSaveMedia_NeverWritesMarkupCharactersInTheFileName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "<img src=x>&weird#chars;here.png"},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil {
		t.Fatalf("SaveMedia = %v", err)
	}

	name := filepath.Base(path)
	for _, c := range []string{"<", ">", "&", "=", "#", ";"} {
		if strings.Contains(name, c) {
			t.Errorf("saved file name %q still contains %q, want it replaced", name, c)
		}
	}
	if want := "_img src_x__weird_chars_here.txt"; name != want {
		t.Errorf("saved file name = %q, want %q", name, want)
	}
}

// TestSaveMedia_FallsBackToAFixedNameWhenTheSenderNameSanitizesAway
// confirms a saved download never ends up named just its extension, or
// nothing at all, when the sender's claimed file name sanitizes to
// nothing usable.
func TestSaveMedia_FallsBackToAFixedNameWhenTheSenderNameSanitizesAway(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: "\x00\x01\u202e.png"},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil || filepath.Base(path) != "file.txt" {
		t.Fatalf("SaveMedia = %q, %v; want file.txt", path, err)
	}
}

// TestSaveMedia_CapsAnOverlongFileName confirms a saved download's own
// name is capped at a sane length, however long a sender's claimed
// file name was.
func TestSaveMedia_CapsAnOverlongFileName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile, FileName: strings.Repeat("a", 300) + ".png"},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}

	stem := strings.TrimSuffix(filepath.Base(path), ".txt")
	if len(stem) > 100 {
		t.Errorf("SaveMedia stem length = %d, want at most 100", len(stem))
	}
}

// TestSaveMedia_FallsBackToAFixedNameWhenThereIsNoFileNameAtAll
// confirms sanitizedStem's own guard for media with no claimed file
// name at all - an ordinary photo, say, which carries none - the same
// as one that sanitizes away to nothing.
func TestSaveMedia_FallsBackToAFixedNameWhenThereIsNoFileNameAtAll(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	path, err := f.commands.SaveMedia(ctx, "m1")
	if err != nil || filepath.Base(path) != "file.txt" {
		t.Fatalf("SaveMedia = %q, %v; want file.txt", path, err)
	}
}

// TestSaveMedia_PropagatesAFetchFailure confirms a message whose media
// fails to download at all is reported as that same failure, never
// masked by a generic save error.
func TestSaveMedia_PropagatesAFetchFailure(t *testing.T) {
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
	f.media.err = errors.New("offline")

	var fetchErr *app.MediaFetchError
	if _, err := f.commands.SaveMedia(ctx, "m1"); !errors.As(err, &fetchErr) {
		t.Errorf("SaveMedia(fetch failure) = %v, want a *MediaFetchError", err)
	}
}

// TestSaveMedia_FailsWhenTheDownloadsDirectoryCannotBeCreated covers
// SaveMedia's own os.MkdirAll failure: here, a plain file already
// sitting where the downloads directory needs to be created.
func TestSaveMedia_FailsWhenTheDownloadsDirectoryCannotBeCreated(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	// A plain file, so the fetch that must succeed before this test's
	// own mkdir failure is reached never has to pass a photo's image
	// check (see app.servedPath).
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	commands, _ := app.New(app.Deps{
		Store: f.store, Dispatcher: f.dispatcher, Notifier: f.notifier, Publisher: f.published,
		Media: f.media, Cache: cache.New(filepath.Join(t.TempDir(), "media"), 1<<20),
		Outgoing: f.outgoing, Clipboard: f.clipboard,
		DownloadsDir: filepath.Join(blocker, "downloads"),
	})

	if _, err := commands.SaveMedia(ctx, "m1"); err == nil {
		t.Error("SaveMedia(downloads directory cannot be created) succeeded, want an error")
	}
}

// TestSaveMedia_FailsWhenTheCopyCannotBeWritten covers copyFile's own
// failure, propagated through SaveMedia: a downloads directory that
// already exists but refuses a new file, here because every write
// permission was removed from it after it was first created.
func TestSaveMedia_FailsWhenTheCopyCannotBeWritten(t *testing.T) {
	t.Parallel()
	skipAsRoot(t)

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	// A plain file, so the fetch that must succeed before this test's
	// own write failure is reached never has to pass a photo's image
	// check (see app.servedPath).
	m := domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[File]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaFile},
	}
	if _, _, err := f.store.AddMessage(ctx, m); err != nil {
		t.Fatal(err)
	}

	readOnly := filepath.Join(t.TempDir(), "downloads")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) }) // let t.TempDir() clean it up

	commands, _ := app.New(app.Deps{
		Store: f.store, Dispatcher: f.dispatcher, Notifier: f.notifier, Publisher: f.published,
		Media: f.media, Cache: cache.New(filepath.Join(t.TempDir(), "media"), 1<<20),
		Outgoing: f.outgoing, Clipboard: f.clipboard,
		DownloadsDir: readOnly,
	})

	if _, err := commands.SaveMedia(ctx, "m1"); err == nil {
		t.Error("SaveMedia(read-only downloads directory) succeeded, want an error")
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
