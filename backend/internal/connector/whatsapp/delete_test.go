package whatsapp

// handleDeleteForMe and DeleteMessages are driven through a fake device,
// the way live_test.go and react_test.go drive this connector's other
// live events and outgoing actions, since the fake is the only way to
// see what was sent or patched without reaching WhatsApp's servers.

import (
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestHandleDeleteForMe_ReportsTheMessageDeleted(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	var sink connectortest.Sink

	c.handleDeleteForMe(t.Context(), &sink, dev, &events.DeleteForMe{
		ChatJID: types.NewJID("15551234567", types.DefaultUserServer), MessageID: "M1",
	})

	if !sink.Has("deleted 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the message reported deleted", sink.Lines())
	}
}

// TestHandleDeleteForMe_ResolvesALIDChatToItsPhoneJID reproduces the
// same mismatch handleRevoke's own LID test does: WhatsApp's own app on
// the phone can report "delete for me" against a chat addressed by its
// hidden id even though every message in it is stored under the phone
// JID.
func TestHandleDeleteForMe_ResolvesALIDChatToItsPhoneJID(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev := newFakeDevice()
	dev.lidPhones = map[string]types.JID{lid.String(): phone}
	var sink connectortest.Sink

	c.handleDeleteForMe(t.Context(), &sink, dev, &events.DeleteForMe{ChatJID: lid, MessageID: "M1"})

	if !sink.Has("deleted 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the LID-addressed delete resolved to the phone JID", sink.Lines())
	}
}

// TestHandleMessage_RevokeAddressedByLIDMatchesAMessageStoredUnderItsPhoneJID
// reproduces a real report: a message arrives (and is stored) addressed
// by phone JID, but the phone later revokes it through a chat addressed
// by WhatsApp's hidden id (LID) for the same contact instead. Without
// resolving that LID back to the phone JID the revoke would be reported
// for a conversation that was never created, and the deleted message
// would keep showing.
func TestHandleMessage_RevokeAddressedByLIDMatchesAMessageStoredUnderItsPhoneJID(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev := newFakeDevice()
	dev.lidPhones = map[string]types.JID{lid.String(): phone}
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: phone, Sender: phone}, ID: "M1", PushName: "Nadia", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hello")},
	})

	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: lid, Sender: lid}, Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M1")}}},
	})

	if !sink.Has("deleted 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the LID-addressed revoke resolved to the phone-addressed conversation the message was stored under", sink.Lines())
	}
}

// TestHandleMessage_RevokeInTheSelfChatMatchesRegardlessOfWhichFormSentIt
// covers the same mismatch for the account's own self-chat: a note sent
// from the phone (addressed by phone JID) revoked by a reply bot on
// another linked device (addressed by LID) must still find it.
func TestHandleMessage_RevokeInTheSelfChatMatchesRegardlessOfWhichFormSentIt(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	phone, lid := types.NewJID("15551234567", types.DefaultUserServer), types.NewJID("111222", types.HiddenUserServer)
	dev := newFakeDevice()
	dev.selfJID, dev.selfLID = phone, lid
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: phone, Sender: phone, IsFromMe: true}, ID: "M1", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("note to self")},
	})

	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: lid, Sender: lid, IsFromMe: true}, Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M1")}}},
	})

	if !sink.Has("deleted 15551234567@s.whatsapp.net M1") {
		t.Errorf("events = %q, want the self-chat revoke resolved to the phone JID the note was stored under", sink.Lines())
	}
}

func TestDeleteMessages_RevokesOurOwnMessageForEveryone(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: "", fromMe: true, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.DeleteMessages(t.Context(), directChat, []string{"M1"}, true); err != nil {
		t.Fatal(err)
	}

	if len(dev.sent) != 1 {
		t.Fatalf("sent = %+v, want one revoke message", dev.sent)
	}
	pm := dev.sent[0].msg.GetProtocolMessage()
	if pm.GetType() != 0 /* REVOKE */ || pm.GetKey().GetID() != "M1" || !pm.GetKey().GetFromMe() {
		t.Errorf("protocol message = %+v, want a revoke of M1, from us", pm)
	}
}

func TestDeleteMessages_RefusesToRevokeSomeoneElsesMessage(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	err := c.DeleteMessages(t.Context(), directChat, []string{"M1"}, true)
	if !errors.Is(err, connector.ErrDeleteUnsupported) {
		t.Errorf("DeleteMessages(forEveryone) on someone else's message = %v, want ErrDeleteUnsupported", err)
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent for a refused revoke", dev.sent)
	}
}

func TestDeleteMessages_RefusesToRevokeAnUnknownMessage(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	err := c.DeleteMessages(t.Context(), directChat, []string{"never-seen"}, true)
	if !errors.Is(err, connector.ErrDeleteUnsupported) {
		t.Errorf("DeleteMessages(forEveryone) on an unknown message = %v, want ErrDeleteUnsupported", err)
	}
}

func TestDeleteMessages_DeletesForMeWithAnAppStatePatch(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	// Someone else's message: "for me" works regardless of who sent it.
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.DeleteMessages(t.Context(), directChat, []string{"M1"}, false); err != nil {
		t.Fatal(err)
	}

	if len(dev.appStatePatches) != 1 {
		t.Fatalf("patches sent = %d, want 1", len(dev.appStatePatches))
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want no revoke message sent for a delete-for-me", dev.sent)
	}
	action := dev.appStatePatches[0].Mutations[0].Value.GetDeleteMessageForMeAction()
	if action == nil {
		t.Errorf("mutation = %+v, want a delete-for-me action", dev.appStatePatches[0].Mutations[0])
	}
	if index := dev.appStatePatches[0].Mutations[0].Index; len(index) != 5 || index[0] != appstate.IndexDeleteMessageForMe || index[2] != "M1" {
		t.Errorf("index = %v, want the index to name M1", index)
	}
}

func TestDeleteMessages_DeletesForMeWithoutAMediaStore(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.DeleteMessages(t.Context(), directChat, []string{"M1"}, false); err != nil {
		t.Fatal(err)
	}
	if len(dev.appStatePatches) != 1 {
		t.Fatalf("patches sent = %d, want 1", len(dev.appStatePatches))
	}
}

func TestDeleteMessages_FailsForABadConversationID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}, []string{"M1"}, true)
	if !errors.Is(err, errBadRemoteID) {
		t.Errorf("DeleteMessages = %v, want errBadRemoteID", err)
	}
}

func TestDeleteMessages_WrapsAWhatsAppError(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = errors.New("server unavailable")
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	if err := c.DeleteMessages(t.Context(), directChat, []string{"M1"}, false); !errors.Is(err, dev.appStateErr) {
		t.Errorf("DeleteMessages = %v, want it to wrap the device's error", err)
	}
}
