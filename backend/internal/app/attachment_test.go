package app_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// writeTestFile writes data to a new file under the test's temp
// directory and returns its path.
func writeTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// writeTestPNG writes a tiny PNG of the given size to a new file and
// returns its path.
func writeTestPNG(t *testing.T, name string, width, height int) string {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}

	return writeTestFile(t, name, buf.Bytes())
}

func TestSend_WithAttachment_StoresMediaAndCopiesTheFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 3, 2)

	m, err := f.commands.Send(ctx, "chat", "look at this", path, "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if m.Media == nil || m.Media.Kind != domain.MediaPhoto || m.Media.Width != 3 || m.Media.Height != 2 {
		t.Fatalf("Media = %+v, want a 3x2 photo", m.Media)
	}
	if m.Media.FileName != "photo.png" || m.Text != "look at this" {
		t.Errorf("message = %+v, want the original name and caption kept", m)
	}

	if m.Media.Path == path {
		t.Errorf("Media.Path = %q, want a copy, not the original path", m.Media.Path)
	}
	if _, err := os.Stat(m.Media.Path); err != nil {
		t.Errorf("stored copy: %v", err)
	}
	if info, err := os.Stat(m.Media.Path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("stored copy mode = %v, %v, want 0600", info, err)
	}

	if !slices.Contains(f.dispatcher.sent, "look at this") {
		t.Errorf("dispatched = %v, want it to include the caption", f.dispatcher.sent)
	}
}

func TestSend_WithAttachment_GivesAPhotoASmallThumbAtOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 400, 300)

	m, err := f.commands.Send(ctx, "chat", "look at this", path, "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if m.Media.Thumb == "" {
		t.Fatal("Media.Thumb is empty, want a small preview the bubble can show before the full photo loads")
	}

	raw, err := base64.StdEncoding.DecodeString(m.Media.Thumb)
	if err != nil {
		t.Fatalf("Thumb is not valid base64: %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Thumb is not a decodable image: %v", err)
	}
	if cfg.Width > 64 || cfg.Height > 64 {
		t.Errorf("Thumb is %dx%d, want it shrunk small", cfg.Width, cfg.Height)
	}
}

func TestSend_WithAttachment_FileGetsNoThumb(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestFile(t, "notes.txt", []byte("plain text file"))

	m, err := f.commands.Send(ctx, "chat", "", path, "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if m.Media.Thumb != "" {
		t.Errorf("Thumb = %q, want none for a plain file", m.Media.Thumb)
	}
}

func TestSend_WithAttachment_PlaceholderCaptionNeverSentAsRealText(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestFile(t, "notes.txt", []byte("plain text file"))

	m, err := f.commands.Send(ctx, "chat", "", path, "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if m.Text != domain.MediaPlaceholder(domain.MediaFile) {
		t.Errorf("Text = %q, want the stored placeholder so the UI has something to show", m.Text)
	}
	if m.Media == nil || m.Media.Kind != domain.MediaFile || m.Media.FileName != "notes.txt" {
		t.Errorf("Media = %+v, want a file attachment named notes.txt", m.Media)
	}
}

func TestSend_RejectsAnUnreadableAttachment(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	if _, err := f.commands.Send(ctx, "chat", "hi", filepath.Join(t.TempDir(), "missing"), ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Send(missing attachment) = %v, want ErrInvalidInput", err)
	}
}

func TestSend_RejectsAnAttachmentOverTheSizeLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	path := filepath.Join(t.TempDir(), "huge.bin")
	sparse, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := sparse.Truncate(app.MaxAttachmentSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := sparse.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = f.commands.Send(ctx, "chat", "hi", path, "")
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("Send(huge attachment) = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "2 GB") {
		t.Errorf("error = %q, want it to mention the 2 GB limit", err)
	}
}

func TestRetry_ResendsTheSameAttachment(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 1, 1)
	f.dispatcher.err = errors.New("offline")

	failed, err := f.commands.Send(ctx, "chat", "hi", path, "")
	if err != nil || failed.Status != domain.StatusFailed {
		t.Fatalf("Send() = %+v, %v", failed, err)
	}

	f.dispatcher.err = nil
	retried, err := f.commands.Retry(ctx, failed.ID)
	if err != nil || retried.Status != domain.StatusPending {
		t.Fatalf("Retry() = %+v, %v", retried, err)
	}

	sentWith := f.dispatcher.lastMedia()
	if sentWith == nil || sentWith.Path == "" {
		t.Fatalf("dispatched media = %+v, want the attachment's path restored", sentWith)
	}
	if _, err := os.Stat(sentWith.Path); err != nil {
		t.Errorf("attachment on retry: %v", err)
	}
}

// TestRetry_FailsClearlyWhenTheAttachmentWasSwept confirms a retry for a
// message whose outgoing copy was already removed - the outgoing area's
// retention sweep dropping it while it waited, say - reports a plain,
// safe error naming what to do, rather than failing deep inside the
// dispatch with nothing useful to say.
func TestRetry_FailsClearlyWhenTheAttachmentWasSwept(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 1, 1)
	f.dispatcher.err = errors.New("offline")

	failed, err := f.commands.Send(ctx, "chat", "hi", path, "")
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

	stored, err := f.store.Message(ctx, failed.ID)
	if err != nil || stored.Status != domain.StatusFailed {
		t.Errorf("stored = %+v, %v; want it still failed, never dispatched", stored, err)
	}
}

// TestDispatch_ReservesTheAttachmentForTheDurationOfTheSend confirms a
// message's attachment survives a sweep that runs while its send is
// still in progress, however small the outgoing area's own size limit,
// and becomes eligible for eviction again only once the send has ended.
func TestDispatch_ReservesTheAttachmentForTheDurationOfTheSend(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.UpsertAccount(ctx, domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.EnsureConversation(ctx, domain.Conversation{
		ID: "chat", AccountID: "wa", RemoteID: "r-chat", Title: "Chat", Kind: domain.KindDirect,
	}); err != nil {
		t.Fatal(err)
	}

	// A size limit of one byte means any unreserved file is evicted the
	// instant Sweep runs.
	outgoing := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), time.Hour, 1)
	dispatcher := &fakeDispatcher{}
	commands, _ := app.New(app.Deps{
		Store: db, Dispatcher: dispatcher, Notifier: &fakeNotifier{}, Publisher: &fakePublisher{},
		Outgoing: outgoing, Clipboard: &fakeClipboard{},
	})

	var reservedDuringSend bool
	dispatcher.onRun = func(m domain.Message) {
		_ = outgoing.Sweep(ctx)
		_, statErr := os.Stat(outgoing.Path(m.ID, m.Media.FileName))
		reservedDuringSend = statErr == nil
	}

	path := writeTestPNG(t, "photo.png", 1, 1)
	sent, err := commands.Send(ctx, "chat", "hi", path, "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !reservedDuringSend {
		t.Error("a sweep mid-send evicted the attachment its own send was still using")
	}

	if err := outgoing.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outgoing.Path(sent.ID, sent.Media.FileName)); !os.IsNotExist(err) {
		t.Error("the attachment was not evicted once its send had ended")
	}
}
