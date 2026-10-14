package whatsapp

// MarkRead is driven through a fake device, as send_test.go's and
// organize_test.go's own tests are. Several of these build a connector
// around a media store a previous "run" already wrote message keys
// into (see newConnectorWithMedia), to prove MarkRead reads a
// conversation's unread messages back from that persisted state rather
// than from anything only this process's own memory ever tracked: that
// is what makes it still work right after a restart or a re-pair.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMarkReadFrom_SkipsASenderIDThisConnectorNeverMade confirms a
// sender id that does not parse as one of this connector's own remote
// ids is skipped rather than failing the whole MarkRead call: senders
// are grouped by whatever message_keys last recorded (see keys.go),
// and a pre-migration row this account sent of its own has none.
func TestMarkReadFrom_SkipsASenderIDThisConnectorNeverMade(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()

	if err := markReadFrom(t.Context(), dev, directPeer, "", []string{"m1"}); err != nil {
		t.Errorf("markReadFrom(bad sender id) = %v, want nil, skipped", err)
	}
	if len(dev.markReadCalls) != 0 {
		t.Errorf("markReadCalls = %v, want none sent for a sender id that does not parse", dev.markReadCalls)
	}
}

// TestMarkReadFrom_WrapsADeviceError confirms a failure sending the
// read receipt itself reaches the caller, rather than being swallowed
// the way an unparseable sender id deliberately is.
func TestMarkReadFrom_WrapsADeviceError(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.markReadErr = errors.New("server unavailable")

	if err := markReadFrom(t.Context(), dev, directPeer, remoteID(directPeer), []string{"m1"}); !errors.Is(err, dev.markReadErr) {
		t.Errorf("markReadFrom = %v, want it to wrap the device's error", err)
	}
}

func TestMarkRead_SendsAReceiptForADirectChatAfterARestart(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media) // a fresh connector: nothing of this run's own in memory

	conv := directChat
	conv.Unread = 1
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 1 || dev.markReadCalls[0].chat != directPeer ||
		len(dev.markReadCalls[0].ids) != 1 || string(dev.markReadCalls[0].ids[0]) != "m1" {
		t.Errorf("markRead calls = %+v, want one receipt naming m1", dev.markReadCalls)
	}
}

// TestMarkRead_SendsNoOtherSignal confirms a MarkRead call's only two
// effects, for a voice note the same as for any other message, are the
// per-message read receipt and the chat-read app-state patch: nothing
// here can ask whatsmeow for a typing indicator or a presence update (the
// device interface above has no such methods), or for a voice note's own
// separate "played" receipt type (markRead takes no receipt-type
// parameter at all, and message_keys, which MarkRead reads unread ids
// back from, has no media-kind column to tell a voice note apart from
// any other message in the first place). Incognito mode (read receipts
// off) skips this whole call from the app layer, proven in
// app.TestMarkRead_SkipsTheServiceWithReadReceiptsOff; this test is what
// proves there was nothing else here for it to have skipped. See
// docs/privacy.md for what that leaves this mode unable to hide.
func TestMarkRead_SendsNoOtherSignal(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "voice1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	conv := directChat
	conv.Unread = 1
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 1 || len(dev.appStatePatches) != 1 {
		t.Errorf("markRead calls = %d, app state patches = %d; want exactly one of each and nothing else",
			len(dev.markReadCalls), len(dev.appStatePatches))
	}
}

func TestMarkRead_SendsOneReceiptPerSenderForAGroupAfterARestart(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	rows := []struct {
		id, sender string
		timestamp  int64
	}{
		{"m1", remoteID(groupMemberA), 1000},
		{"m2", remoteID(groupMemberA), 2000},
		{"m3", remoteID(groupMemberB), 3000},
	}
	for _, r := range rows {
		if err := media.putMessageKey(t.Context(), groupChat.RemoteID, r.id, messageKey{senderID: r.sender, fromMe: false, timestamp: r.timestamp}); err != nil {
			t.Fatal(err)
		}
	}

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	conv := groupChat
	conv.Unread = 3
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 2 {
		t.Fatalf("markRead calls = %d, want 2, one per sender: %+v", len(dev.markReadCalls), dev.markReadCalls)
	}
	for _, call := range dev.markReadCalls {
		if call.chat != groupJID {
			t.Errorf("call chat = %v, want the group JID", call.chat)
		}
	}
}

func TestMarkRead_SendsTheChatReadAppStatePatch(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	conv := directChat
	conv.Unread = 1
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.appStatePatches) != 1 {
		t.Fatalf("app state patches sent = %d, want 1", len(dev.appStatePatches))
	}
	mutation := dev.appStatePatches[0].Mutations[0]
	action := mutation.Value.GetMarkChatAsReadAction()
	if action == nil || !action.GetRead() {
		t.Errorf("mutation = %+v, want a mark-chat-as-read action with Read true", mutation)
	}
	if index := mutation.Index; len(index) != 2 || index[0] != appstate.IndexMarkChatAsRead || index[1] != directChat.RemoteID {
		t.Errorf("index = %v, want [%s %s]", index, appstate.IndexMarkChatAsRead, directChat.RemoteID)
	}
}

func TestMarkRead_IgnoresAFailedAppStatePatch(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()
	dev.appStateErr = errors.New("server unavailable")
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	conv := directChat
	conv.Unread = 1
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Errorf("MarkRead = %v, want a failed app-state patch to be ignored, not fail the call", err)
	}
	if len(dev.markReadCalls) != 1 {
		t.Errorf("markRead calls = %d, want the per-message receipt still sent", len(dev.markReadCalls))
	}
}

func TestMarkRead_NoopWithNothingUnread(t *testing.T) {
	t.Parallel()

	dev, _, c, _ := connectedMediaFixture(t)

	if err := c.MarkRead(t.Context(), directChat); err != nil { // directChat.Unread is 0
		t.Fatal(err)
	}
	if len(dev.markReadCalls) != 0 {
		t.Errorf("markRead calls = %d, want none", len(dev.markReadCalls))
	}
	if len(dev.appStatePatches) != 0 {
		t.Errorf("app state patches = %d, want none", len(dev.appStatePatches))
	}
}

func TestMarkRead_NoopWithoutAMediaStore(t *testing.T) {
	t.Parallel()

	dev, _, c := connectedFixture(t) // built with no media store

	conv := directChat
	conv.Unread = 5
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}
	if len(dev.markReadCalls) != 0 {
		t.Errorf("markRead calls = %d, want none without a media store to read from", len(dev.markReadCalls))
	}
}

func TestMarkRead_FailsBeforeConnecting(t *testing.T) {
	t.Parallel()

	c := newTestConnector(newFakeDevice())
	if err := c.MarkRead(t.Context(), directChat); !errors.Is(err, errNotConnected) {
		t.Errorf("MarkRead = %v, want errNotConnected", err)
	}
}

// TestMarkRead_PicksTheNewestOfAHistorySyncedConversationsMessages
// seeds message_keys through a real history sync instead of writing
// rows by hand, to prove the two paths that fill it (a live message
// and a synced one) feed MarkRead identically.
func TestMarkRead_PicksTheNewestOfAHistorySyncedConversationsMessages(t *testing.T) {
	t.Parallel()

	dev, sink, c, media := connectedMediaFixture(t)

	synced := &waHistorySync.Conversation{
		ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia"), UnreadCount: u32(1),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "older", false), historyMsg("H2", "newest", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(synced))

	// The service's own unread count for the conversation, 1 here, is
	// what tells MarkRead how many of its newest saved messages to
	// pick; nothing from this sync is queued inside the connector
	// itself for it to find later (see markread.go).
	conv := domain.Conversation{RemoteID: "15551234567@s.whatsapp.net", Unread: 1}
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if len(dev.markReadCalls) != 1 || len(dev.markReadCalls[0].ids) != 1 || string(dev.markReadCalls[0].ids[0]) != "H2" {
		t.Errorf("markRead calls = %+v, want one call for H2, the newest of the one unread message", dev.markReadCalls)
	}
}

// TestSendChatReadState_ReportsSkippedWithNoSavedMessage confirms a
// conversation with nothing saved in message_keys at all is reported as
// skipped, the diagnostic line's way of telling "nothing to patch" apart
// from "WhatsApp refused the patch".
func TestSendChatReadState_ReportsSkippedWithNoSavedMessage(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	dev := newFakeDevice()

	got := sendChatReadState(t.Context(), dev, media, directPeer, directChat.RemoteID)
	if got.status != chatPatchSkipped {
		t.Errorf("sendChatReadState = %+v, want status %q", got, chatPatchSkipped)
	}
	if len(dev.appStatePatches) != 0 {
		t.Errorf("appStatePatches = %d, want none sent", len(dev.appStatePatches))
	}
}

// TestSendChatReadState_ReportsNoTimestampForAPreMigrationRow confirms a
// message saved before keys.go's timestamp column existed (timestamp 0,
// its default) still gets a patch sent, but is reported as carrying no
// real timestamp, so the diagnostic line can tell apart an account that
// still needs a fresh message before MarkRead's app-state patch truly
// identifies one.
func TestSendChatReadState_ReportsNoTimestampForAPreMigrationRow(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{senderID: remoteID(directPeer)}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()

	got := sendChatReadState(t.Context(), dev, media, directPeer, directChat.RemoteID)
	if got.status != chatPatchSent || got.hasTimestamp {
		t.Errorf("sendChatReadState = %+v, want it sent with hasTimestamp false", got)
	}
}

// TestSendChatReadState_AddressesTheMigratedChatByItsLID confirms a
// chat first known by its phone JID, once whatsmeow's own LID store
// has since learned that chat's LID, gets its "mark chat as read"
// app-state patch addressed by that LID instead: a real account showed
// WhatsApp's server reject this patch every single time while the
// chat's own phone-JID address was used, the symptom lidFormOf (see
// normalize.go) exists to fix.
func TestSendChatReadState_AddressesTheMigratedChatByItsLID(t *testing.T) {
	t.Parallel()

	lid := types.NewJID("98765432109", types.HiddenUserServer)
	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()
	dev.lidPhones = map[string]types.JID{lid.String(): directPeer}

	got := sendChatReadState(t.Context(), dev, media, directPeer, directChat.RemoteID)
	if got.status != chatPatchSent {
		t.Fatalf("sendChatReadState = %+v, want it sent", got)
	}
	if len(dev.appStatePatches) != 1 {
		t.Fatalf("appStatePatches = %d, want 1", len(dev.appStatePatches))
	}

	mutation := dev.appStatePatches[0].Mutations[0]
	if index := mutation.Index; len(index) != 2 || index[1] != lid.String() {
		t.Errorf("index = %v, want the chat addressed by its LID %s", index, lid.String())
	}

	action := mutation.Value.GetMarkChatAsReadAction()
	messages := action.GetMessageRange().GetMessages()
	if len(messages) != 1 || messages[0].GetKey().GetRemoteJID() != lid.String() {
		t.Errorf("message key remote JID = %+v, want the chat's LID %s", messages, lid.String())
	}
}

// TestSendChatReadState_KeepsThePhoneJIDWhenNoLIDIsKnown confirms a
// chat whatsmeow's LID store has no mapping for at all keeps being
// addressed exactly as it always was, so an account that has not
// migrated to LID addressing sees no change in behaviour.
func TestSendChatReadState_KeepsThePhoneJIDWhenNoLIDIsKnown(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice() // lidPhones deliberately left unset

	got := sendChatReadState(t.Context(), dev, media, directPeer, directChat.RemoteID)
	if got.status != chatPatchSent {
		t.Fatalf("sendChatReadState = %+v, want it sent", got)
	}

	mutation := dev.appStatePatches[0].Mutations[0]
	if index := mutation.Index; len(index) != 2 || index[1] != directChat.RemoteID {
		t.Errorf("index = %v, want the chat still addressed by its phone JID %s", index, directChat.RemoteID)
	}
}

// TestChatPatchFailureClass_SortsEveryAppStateErrorIntoItsSafeCategory
// confirms the category logMarkRead reports for a failed chat-read
// patch never reflects anything beyond these four safe words.
func TestChatPatchFailureClass_SortsEveryAppStateErrorIntoItsSafeCategory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want string
	}{
		"no app-state key yet":   {appstate.ErrKeyNotFound, "key"},
		"the server rejected it": {whatsmeow.ErrAppStateUpdate, "rejected"},
		"a timed-out context":    {context.DeadlineExceeded, "timeout"},
		"a cancelled context":    {context.Canceled, "timeout"},
		"a plain network error":  {errors.New("dial tcp: connection refused"), "other"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := chatPatchFailureClass(tt.err); got != tt.want {
				t.Errorf("chatPatchFailureClass(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestMarkRead_LogsTheChatPatchFailureCategory confirms a failed
// app-state patch's diagnostic line names why, not just that it failed,
// so a report of "still shows unread on the phone" can be checked
// against which of the known causes it actually was. Not run with
// t.Parallel so capturing the log output stays reliable.
func TestMarkRead_LogsTheChatPatchFailureCategory(t *testing.T) {
	buf := captureLog(t)

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()
	dev.appStateErr = appstate.ErrKeyNotFound
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	conv := directChat
	conv.Unread = 1
	if err := c.MarkRead(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "chat_patch=failed, chat_patch_error=key") {
		t.Errorf("log output = %q, want the chat_patch_error category included", got)
	}
}

func TestMarkRead_FailsForABadConversationID(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	if err := media.putMessageKey(t.Context(), "not-a-jid", "m1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	conv := domain.Conversation{RemoteID: "not-a-jid", Unread: 1}
	if err := c.MarkRead(t.Context(), conv); !errors.Is(err, errBadRemoteID) {
		t.Errorf("MarkRead = %v, want errBadRemoteID", err)
	}
}
