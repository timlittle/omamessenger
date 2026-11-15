package telegram

// sync.go's own handlers are already covered against connectortest.Sink
// in sync_test.go; these tests drive the same handlers through a real
// app.Ingest over a real SQLite store instead (see newRealIngest), so a
// chat or message that the sink would record but the store would
// actually refuse - the way the Signal titling bug let a message
// vanish - is caught here. Split out of sync_test.go to stay under its
// own file-length limit.

import (
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

// TestSync_StoresAChatImportedFromTheDialogSync drives a dialog sync -
// distinct from a live message, the path that recovers a chat the live
// update never reached (see TestSync_RecoversAChatTheLiveUpdateNeverReached)
// - through a real app.Ingest over a real store, so this proves an
// imported chat and its message are actually kept, not just recorded by
// connectortest.Sink's own simplified bookkeeping (see realIngest's own
// doc comment).
func TestSync_StoresAChatImportedFromTheDialogSync(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}, TopMessage: 9, UnreadCount: 1},
		},
		Messages: []tg.MessageClass{
			&tg.Message{ID: 9, PeerID: &tg.PeerUser{UserID: 42}, Message: "hello from the sync"},
		},
		Users: []tg.UserClass{nadia()},
	})
	f.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{&tg.Message{ID: 9, PeerID: &tg.PeerUser{UserID: 42}, Message: "hello from the sync"}},
	})

	c := realConnector()
	ingest := newRealIngest(t)
	if err := c.sync(t.Context(), tg.NewClient(f), ingest); err != nil {
		t.Fatal(err)
	}

	stored, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, "user:42:99")
	if err != nil {
		t.Fatalf("ConversationByRemote() = %v", err)
	}
	if stored.Title != "Nadia" {
		t.Errorf("conversation title = %q, want %q", stored.Title, "Nadia")
	}

	messages, _, err := ingest.store.Messages(t.Context(), stored.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].Text != "hello from the sync" {
		t.Fatalf("messages = %+v, %v; want the synced message stored", messages, err)
	}
}

// TestHistory_BeforeConversationReported_DropsAndLogs checks that a
// page of history reported through history() before its conversation
// exists in the store - never reachable through listDialogs's own
// sink.Conversation call first, but reachable if a future caller skips
// it, the way this test does on purpose by calling history() directly
// - is dropped by the real store rather than stored unreachably, and
// that the drop is logged rather than silent, without naming the
// conversation's own remote id (see privacy.md).
func TestHistory_BeforeConversationReported_DropsAndLogs(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{&tg.Message{ID: 9, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"}},
	})

	ingest := newRealIngest(t)
	c := connectedTo(f, ingest)

	if _, err := c.history(t.Context(), tg.NewClient(f), ingest, page{conv: chatWithNadia, limit: 10}); err != nil {
		t.Fatal(err)
	}

	if _, err := ingest.store.ConversationByRemote(t.Context(), realAccountID, chatWithNadia.RemoteID); err == nil {
		t.Fatal("ConversationByRemote() succeeded for a conversation never reported")
	}

	lines := ingest.logger.take()
	if len(lines) != 1 {
		t.Fatalf("logged lines = %v, want exactly one drop logged", lines)
	}
	if strings.Contains(lines[0], "42") {
		t.Errorf("logged line = %q, want it never to name the conversation's own remote id", lines[0])
	}
}
