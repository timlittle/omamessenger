package app_test

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestIncoming_ReportsReadToTheServiceWhenFocused reproduces messages
// piling up unread in a conversation the user is actively chatting in:
// marking it read locally is not enough, because the service's own
// unread count, synced back later through Unread, would otherwise still
// be non-zero and resurrect the badge. A burst of messages while focused
// must still produce only one MarkRead call to the service.
func TestIncoming_ReportsReadToTheServiceWhenFocused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
			t.Fatal(err)
		}

		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "first"))
		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-2", "second"))

		if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
			t.Errorf("unread = %d, want 0", got.Unread)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if got := f.dispatcher.read; !slices.Equal(got, []string{chat.ID}) {
			t.Errorf("read receipts = %v, want one report for %s", got, chat.ID)
		}
	})
}

// TestIncoming_ReportsHowManyWereUnreadToTheService confirms the
// conversation handed to the service's debounced MarkRead carries the
// number of messages this burst actually left unread for it to report,
// not a stale count from before any of them arrived: a service such as
// WhatsApp's connector needs that number to know how many of the
// conversation's newest messages to send a read receipt for.
func TestIncoming_ReportsHowManyWereUnreadToTheService(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
			t.Fatal(err)
		}

		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "first"))
		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-2", "second"))

		time.Sleep(time.Second)
		synctest.Wait()

		if len(f.dispatcher.readConv) != 1 || f.dispatcher.readConv[0].Unread != 2 {
			t.Fatalf("dispatcher saw %+v, want one call with Unread 2", f.dispatcher.readConv)
		}
	})
}

// TestIncoming_SkipsTheDebouncedReadReceiptWithReadReceiptsOff checks
// incognito read receipts on the debounced path, not just the direct
// Commands.MarkRead one: with the setting off, a burst of messages while
// focused still clears the local unread count but never reaches the
// dispatcher, even after the debounce window passes.
func TestIncoming_SkipsTheDebouncedReadReceiptWithReadReceiptsOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		f.commands.ApplySettings(app.Settings{ReadReceipts: false})
		if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
			t.Fatal(err)
		}

		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "first"))
		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-2", "second"))

		if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
			t.Errorf("unread = %d, want 0 even with read receipts off", got.Unread)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if len(f.dispatcher.read) != 0 {
			t.Errorf("read receipts = %v, want none reaching the service", f.dispatcher.read)
		}
	})
}

// TestIncoming_NeverReportsReadWhenNotLookingAtIt checks the service is
// never told a conversation was read while the user is not looking at
// it, even after the debounce window a focused read would use has passed.
func TestIncoming_NeverReportsReadWhenNotLookingAtIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("in-1", "first"))

		time.Sleep(time.Second)
		synctest.Wait()

		if len(f.dispatcher.read) != 0 {
			t.Errorf("read receipts = %v, want none", f.dispatcher.read)
		}
	})
}

// TestUnread_ReMarksReadForTheOpenConversation reproduces the badge
// reappearing on a conversation the user is still looking at: Telegram's
// own count can report non-zero for it, for example because our read
// report is still in flight. That must not be shown, and the service
// should be told again rather than left out of step.
func TestUnread_ReMarksReadForTheOpenConversation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx := t.Context()
		chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

		if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
			t.Fatal(err)
		}

		f.ingest.Unread(ctx, "wa", chat.RemoteID, 3)

		if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
			t.Errorf("unread = %d, want 0", got.Unread)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if got := f.dispatcher.read; !slices.Equal(got, []string{chat.ID}) {
			t.Errorf("read receipts = %v, want one report for %s", got, chat.ID)
		}
	})
}
