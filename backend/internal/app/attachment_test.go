package app_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

	m, err := f.commands.Send(ctx, "chat", "look at this", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
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

	m, err := f.commands.Send(ctx, "chat", "look at this", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
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

	m, err := f.commands.Send(ctx, "chat", "", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
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

	m, err := f.commands.Send(ctx, "chat", "", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
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

	if _, err := f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: filepath.Join(t.TempDir(), "missing"), ReplyToID: "", Mentions: nil}); !errors.Is(err, app.ErrInvalidInput) {
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

	_, err = f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path, ReplyToID: "", Mentions: nil})
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("Send(huge attachment) = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "2 GB") {
		t.Errorf("error = %q, want it to mention the 2 GB limit", err)
	}
}

// sentFailedAttachment builds a fixture, attaches a 1x1 PNG to a new
// message in a chat called "chat" and makes the dispatcher fail so the
// send ends up Failed, for a retry test that goes on to recover it or
// fail it further.
func sentFailedAttachment(t *testing.T) (f *fixture, ctx context.Context, path string, failed domain.Message) {
	t.Helper()

	f = newFixture(t, false)
	ctx = t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path = writeTestPNG(t, "photo.png", 1, 1)
	f.dispatcher.err = errors.New("offline")

	var err error
	failed, err = f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path})
	if err != nil || failed.Status != domain.StatusFailed {
		t.Fatalf("Send() = %+v, %v", failed, err)
	}

	return f, ctx, path, failed
}

func TestRetry_ResendsTheSameAttachment(t *testing.T) {
	t.Parallel()

	f, ctx, _, failed := sentFailedAttachment(t)

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

// TestRetry_RecopiesFromTheOriginalWhenTheOutgoingCopyIsMissing confirms
// a retry whose outgoing copy has gone missing - some other process
// removed it, say - falls back to re-copying the file it was attached
// from, as long as that file still exists and still looks like the same
// one, rather than failing.
func TestRetry_RecopiesFromTheOriginalWhenTheOutgoingCopyIsMissing(t *testing.T) {
	t.Parallel()

	f, ctx, _, failed := sentFailedAttachment(t)

	if err := f.outgoing.Remove(ctx, failed.ID, failed.Media.FileName); err != nil {
		t.Fatal(err)
	}

	f.dispatcher.err = nil
	retried, err := f.commands.Retry(ctx, failed.ID)
	if err != nil || retried.Status != domain.StatusPending {
		t.Fatalf("Retry() = %+v, %v, want it re-copied and sent", retried, err)
	}

	if _, err := os.Stat(f.outgoing.Path(failed.ID, failed.Media.FileName)); err != nil {
		t.Errorf("outgoing copy after retry: %v, want it re-copied from the original", err)
	}
}

// TestRetry_FailsClearlyWhenNeitherCopyIsAvailable confirms a retry
// whose outgoing copy is missing and whose original file is also gone
// reports a plain, safe error naming what to do, rather than failing
// deep inside the dispatch with nothing useful to say.
func TestRetry_FailsClearlyWhenNeitherCopyIsAvailable(t *testing.T) {
	t.Parallel()

	f, ctx, path, failed := sentFailedAttachment(t)

	if err := f.outgoing.Remove(ctx, failed.ID, failed.Media.FileName); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
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

// TestRetry_FailsClearlyWhenTheOriginalHasChanged confirms a retry
// whose outgoing copy is missing and whose original file has changed
// size since it was attached - a different file now sitting at the
// same path - is treated the same as the original being gone: a plain,
// safe error, never trusting the changed file enough to send it.
func TestRetry_FailsClearlyWhenTheOriginalHasChanged(t *testing.T) {
	t.Parallel()

	f, ctx, path, failed := sentFailedAttachment(t)

	if err := f.outgoing.Remove(ctx, failed.ID, failed.Media.FileName); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("a completely different file now"), 0o600); err != nil {
		t.Fatal(err)
	}

	f.dispatcher.err = nil
	if _, err := f.commands.Retry(ctx, failed.ID); !errors.Is(err, app.ErrInvalidInput) ||
		!strings.Contains(err.Error(), "attach it again") {
		t.Fatalf("Retry() error = %v, want an invalid-input error about attaching it again", err)
	}
}

// TestDispatch_ReservesTheAttachmentForTheDurationOfTheSend confirms a
// message's attachment survives a sweep that runs while its send is
// still in progress, even though its own message row briefly looks
// like an orphan to a store that has not committed it yet, and remains
// after the send ends since it still belongs to a real, pending
// message.
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

	// neverExists always reports the file an orphan, like a message
	// whose exists check cannot find it yet; only Reserve keeps Sweep
	// from acting on that during the send.
	outgoing := cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 0, 1<<30,
		func(context.Context, string) (bool, error) { return false, nil })
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
	sent, err := commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !reservedDuringSend {
		t.Error("a sweep mid-send removed the attachment its own send was still using")
	}

	if err := outgoing.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outgoing.Path(sent.ID, sent.Media.FileName)); !os.IsNotExist(err) {
		t.Error("the attachment was not removed once its send had ended and it still looked orphaned")
	}
}
