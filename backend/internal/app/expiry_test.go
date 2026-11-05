package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// runExpirySweeper starts commands.RunExpirySweeper in its own
// goroutine, returning a function that cancels it and waits for it to
// stop, so every test leaves no goroutine running past its own end.
func runExpirySweeper(ctx context.Context, cancel context.CancelFunc, commands *app.Commands) func() {
	var wg sync.WaitGroup
	wg.Go(func() { commands.RunExpirySweeper(ctx) })

	return func() {
		cancel()
		wg.Wait()
	}
}

// TestRunExpirySweeper_RemovesAnOutgoingMessageOnceItsPeriodRunsOut
// checks a WhatsApp-style disappearing message, whose timer counts
// from sending: once its period runs out, its text, its media and the
// search entry alongside it are gone, and the UI is told as if it had
// been deleted.
func TestRunExpirySweeper_RemovesAnOutgoingMessageOnceItsPeriodRunsOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		conv := f.conversation(t, "chat", "Chat", domain.KindDirect)

		path, err := f.outgoing.Store(ctx, "m1", "photo.jpg", strings.NewReader("bytes"))
		if err != nil {
			t.Fatalf("store outgoing attachment: %v", err)
		}

		now := time.Now().UnixMilli()
		if _, _, err := f.store.AddMessage(ctx, domain.Message{
			ID: "m1", ConversationID: conv.ID, RemoteID: "r1", Text: "gone soon", Outgoing: true,
			Status: domain.StatusSent, Created: now, ExpireAfter: 5,
			Media: &domain.Media{Kind: domain.MediaPhoto, FileName: "photo.jpg"},
		}); err != nil {
			t.Fatalf("add message: %v", err)
		}

		stop := runExpirySweeper(ctx, cancel, f.commands)
		defer stop()
		synctest.Wait()

		time.Sleep(6 * time.Second)
		synctest.Wait()

		if _, err := f.store.Message(ctx, "m1"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Message() err = %v, want ErrNotFound once the message expired", err)
		}

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("outgoing attachment still at %s after its message expired", path)
		}

		removed, ok := f.published.lastOf(app.EventMessageRemoved).(app.MessageRemoved)
		if !ok || removed.MessageID != "m1" {
			t.Errorf("published message.removed = %+v, %t, want m1", removed, ok)
		}
	})
}

// TestRunExpirySweeper_RemovesAnIncomingMessagesDownloadedMedia checks
// that an incoming disappearing message's own downloaded media file is
// removed from the media cache once it expires, not just an outgoing
// attachment: the brief promises text, media and search entry are all
// gone.
func TestRunExpirySweeper_RemovesAnIncomingMessagesDownloadedMedia(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		conv := f.conversation(t, "chat", "Chat", domain.KindDirect)

		if err := os.MkdirAll(f.mediaDir, 0o700); err != nil {
			t.Fatal(err)
		}
		cached := filepath.Join(f.mediaDir, "m1.photo.jpg")
		if err := os.WriteFile(cached, []byte("bytes"), 0o600); err != nil {
			t.Fatal(err)
		}

		now := time.Now().UnixMilli()
		if _, _, err := f.store.AddMessage(ctx, domain.Message{
			ID: "m1", ConversationID: conv.ID, RemoteID: "r1", Text: "gone soon",
			Status: domain.StatusReceived, Created: now, ExpireAfter: 5,
			Media: &domain.Media{Kind: domain.MediaPhoto, FileName: "photo.jpg"},
		}); err != nil {
			t.Fatalf("add message: %v", err)
		}

		stop := runExpirySweeper(ctx, cancel, f.commands)
		defer stop()
		synctest.Wait()

		time.Sleep(6 * time.Second)
		synctest.Wait()

		if _, err := os.Stat(cached); !os.IsNotExist(err) {
			t.Errorf("downloaded media still at %s after its message expired", cached)
		}
	})
}

// TestFetchMedia_NeverFetchesAViewOnceMessage checks that a view-once
// message, which a connector reports with no media at all (see
// whatsapp.message and telegram.message), is refused rather than
// fetched: FetchMedia never even reaches the connector.
func TestFetchMedia_NeverFetchesAViewOnceMessage(t *testing.T) {
	f := newFixture(t, false)
	ctx := t.Context()
	conv := f.conversation(t, "chat", "Chat", domain.KindDirect)

	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: conv.ID, RemoteID: "r1",
		Text: "View-once photo: open it on your phone", ViewOnce: true,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	if _, _, _, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false); !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("FetchMedia() err = %v, want ErrInvalidInput", err)
	}

	if len(f.media.fetched) != 0 {
		t.Errorf("fetched = %v, want the connector never asked for a view-once message", f.media.fetched)
	}
}

// TestRunExpirySweeper_KeepsAPendingExpiryAcrossARestart checks that a
// message armed before a restart is still swept once its period runs
// out, by reopening the same database rather than relying on anything
// kept in memory.
func TestRunExpirySweeper_KeepsAPendingExpiryAcrossARestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		conv := f.conversation(t, "chat", "Chat", domain.KindDirect)

		now := time.Now().UnixMilli()
		if _, _, err := f.store.AddMessage(ctx, domain.Message{
			ID: "m1", ConversationID: conv.ID, RemoteID: "r1", Text: "gone soon",
			Outgoing: true, Status: domain.StatusSent, Created: now, ExpireAfter: 5,
		}); err != nil {
			t.Fatalf("add message: %v", err)
		}

		// A restart rebuilds the application over the same store, the
		// same way main.go does, with nothing about the pending expiry
		// carried over except what the database itself holds.
		restarted, _, _, _ := appOver(t, f.store)
		runCtx, cancel := context.WithCancel(t.Context())
		stop := runExpirySweeper(runCtx, cancel, restarted)
		defer stop()
		synctest.Wait()

		time.Sleep(6 * time.Second)
		synctest.Wait()

		if _, err := f.store.Message(ctx, "m1"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Message() err = %v, want ErrNotFound once the message expired", err)
		}
	})
}
