package whatsapp

// organize_sync_test.go holds the tests for how a pin, archive or
// mark-chat-as-read mutation that arrives some way other than this
// connect's own SetPinned/SetArchived call - a history resync, a live
// echo from the phone, or the phone's own mark-chat-as-read mutation -
// interacts with what this process already knows: split out of
// organize_test.go, which keeps SetPinned and SetArchived's own direct
// tests plus the fakeDevice methods both files share, once that file
// passed revive's file-length gate (the same reasoning
// conversations_archive_test.go's split in the app package already
// used).

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

func TestSyncConversation_NeverRevertsAPinJustSetLocally(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}
	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Fatalf("events = %q, want the pin reported right after SetPinned", sink.Lines())
	}
	sink.Take()

	// A history resync reports this same chat with its own sync blob,
	// which never carries pinned or archived at all any more (see
	// docs/decisions.md): this must not revert the pin, because
	// reportSyncedOrganize never reads either from the blob to begin
	// with, only from whatsmeow's own chat settings.
	media := newTestMediaStore(t)
	conv := &waHistorySync.Conversation{
		ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the just-set pin kept rather than reverted by a stale resync", sink.Lines())
	}
}

func TestHandlePin_LiveEchoStillOverridesALocalPin(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}
	sink.Take()

	// The phone itself unpinned the chat: whatsmeow's own chat settings
	// store already reflects that by the time the live echo fires (see
	// docs/decisions.md), so seeding it here stands in for that, and a
	// live echo must always be trusted, even over a pin this process
	// set a moment ago.
	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	dev.setChatSettings(jid.String(), false, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(false)}})

	if !sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the live unpin to take effect", sink.Lines())
	}
}

func TestSyncConversation_AppliesAPinThatArrivedBeforeTheConversationExisted(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	// The phone's pin arrives as a live app-state echo, already applied
	// to whatsmeow's own chat settings store, before history sync has
	// ever reported this conversation. A real app drops this first
	// report, since it has no conversation to attach it to yet, but
	// whatsmeow's store still remembers the pin regardless.
	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	dev.setChatSettings(jid.String(), true, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	sink.Take()

	// History sync now creates the conversation. Its own synced
	// snapshot carries no pin timestamp for it, since the pin lives
	// only in app state: reading whatsmeow's chat settings instead of
	// this sync's own fields is what still finds it pinned.
	media := newTestMediaStore(t)
	conv := &waHistorySync.Conversation{
		ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Errorf("events = %q, want the early pin reported once the conversation exists", sink.Lines())
	}
}

func TestSyncConversation_NeverRevertsAPinKnownFromAppState(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	media := newTestMediaStore(t)
	syncIt := func() {
		conv := &waHistorySync.Conversation{
			ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
			Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
		}
		c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))
	}

	// The conversation already exists, from an earlier sync, and only
	// afterwards does the phone's pin arrive live.
	syncIt()
	dev.setChatSettings(jid.String(), true, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	sink.Take()

	// A later resync still carries nothing of its own about pinned or
	// archived: the pin confirmed live a moment ago survives it, since
	// it is read fresh from whatsmeow's chat settings every time.
	syncIt()

	if sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the app-state pin kept rather than reverted by a later resync", sink.Lines())
	}
}

func TestSyncConversation_AppliesAnArchiveThatArrivedBeforeTheConversationExisted(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	dev.setChatSettings(jid.String(), false, true)
	c.handleArchive(t.Context(), sink, dev, nil, &events.Archive{JID: jid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(true)}})
	sink.Take()

	// History sync's own blob carries nothing about archived either:
	// this must not undo an archive already confirmed live.
	media := newTestMediaStore(t)
	conv := &waHistorySync.Conversation{
		ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("organized " + directChat.RemoteID + " false true") {
		t.Errorf("events = %q, want the early archive reported once the conversation exists", sink.Lines())
	}
}

func TestHandlePinAndArchive_ApplyToAPhoneKeyedConversationAddressedByLID(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	// WhatsApp can address the same chat by the phone JID it is already
	// stored under or by its LID interchangeably; whatsmeow's own chat
	// settings store is keyed by whichever form the app-state mutation
	// actually named, here the LID, the same as a pin or archive sent
	// by LID really would be. Either way it must land on the
	// phone-keyed conversation, not a separate one.
	dev.setChatSettings(lid.String(), true, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: lid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	if !sink.Has("organized " + remoteID(phone) + " true false") {
		t.Errorf("events = %q, want the LID-addressed pin applied to the phone-keyed conversation", sink.Lines())
	}

	dev.setChatSettings(lid.String(), true, true)
	c.handleArchive(t.Context(), sink, dev, nil, &events.Archive{JID: lid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(true)}})
	if !sink.Has("organized " + remoteID(phone) + " true true") {
		t.Errorf("events = %q, want the LID-addressed archive applied to the phone-keyed conversation", sink.Lines())
	}
}

func TestConnector_UnpinsOnArchive(t *testing.T) {
	t.Parallel()

	_, _, c := connectedFixture(t)
	if !c.UnpinsOnArchive() {
		t.Error("UnpinsOnArchive() = false, want true: WhatsApp always unpins a chat it archives")
	}

	var _ connector.ArchiveUnpinner = c
}

func TestSetArchived_TimesOutWhenWhatsAppNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, _, c := connectedFixture(t)
		dev.appStateBlocks = true

		done := make(chan error, 1)
		go func() { done <- c.SetArchived(t.Context(), directChat, true) }()

		time.Sleep(organizeTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("SetArchived = %v, want context.DeadlineExceeded after the organize timeout", err)
			}
		default:
			t.Fatal("SetArchived did not return once its timeout elapsed")
		}
	})
}

func TestHandleMarkChatAsRead_ReportsUnreadZeroWhenMarkedRead(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handleMarkChatAsRead(t.Context(), sink, dev, nil, &events.MarkChatAsRead{
		JID: jid, Action: &waSyncAction.MarkChatAsReadAction{Read: boolPtr(true)},
	})

	if !sink.Has("unread 15551234567@s.whatsapp.net 0") {
		t.Errorf("events = %q, want unread reset to 0, the same as the self-read receipt lane", sink.Lines())
	}
}

func TestHandleMarkChatAsRead_IgnoresAMarkedUnreadChange(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handleMarkChatAsRead(t.Context(), sink, dev, nil, &events.MarkChatAsRead{
		JID: jid, Action: &waSyncAction.MarkChatAsReadAction{Read: boolPtr(false)},
	})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a chat marked unread", sink.Lines())
	}
}
