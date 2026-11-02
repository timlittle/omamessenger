package whatsapp

// Split out of live_test.go to stay under its own file-length limit:
// handleRevoke's and handleEdit's authorization check (see
// authorize.go) end to end through handleMessage, covering both the
// authorized case and the forged cases from Vuln 1. authorize_test.go
// covers authorizedActor itself, in isolation.

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// TestHandleMessage_RevokeAndEdit covers the authorized case: Nadia,
// who originally sent M0 (seeded below exactly as handleContent would
// have saved it), revoking then editing her own message. See
// TestHandleMessage_RefusesARevokeFromSomeoneElse and
// TestHandleMessage_RefusesAnEditFromSomeoneElse for the forged cases
// this check exists to stop.
func TestHandleMessage_RevokeAndEdit(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	seedMessageKey(t, media, "15551234567@s.whatsapp.net", "M0", messageKey{senderID: "15551234567@s.whatsapp.net", fromMe: false})

	revokeMsg := &events.Message{
		Info:    liveInfo(),
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")}}},
	}
	c.handleMessage(t.Context(), sink, dev, media, revokeMsg)
	if !sink.Has("deleted 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want the revoked message reported deleted", sink.Lines())
	}

	editMsg := &events.Message{
		Info: liveInfo(),
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")},
			EditedMessage: &waE2E.Message{Conversation: strPtr("corrected")},
		}},
	}
	c.handleMessage(t.Context(), sink, dev, media, editMsg)
	if !sink.Has("edited 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want the edit reported", sink.Lines())
	}
}

// TestHandleMessage_RefusesARevokeFromSomeoneElse covers Vuln 1: Mallory,
// a different member of the chat from Nadia (M0's real sender, seeded
// below), sends a revoke for M0. Without authorizedActor's check,
// anyone in the chat could delete anyone else's message this way.
func TestHandleMessage_RefusesARevokeFromSomeoneElse(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	seedMessageKey(t, media, "15551234567@s.whatsapp.net", "M0", messageKey{senderID: "15551234567@s.whatsapp.net", fromMe: false})

	mallory := types.NewJID("15559876543", types.DefaultUserServer)
	forgedRevoke := &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{
			Chat: types.NewJID("15551234567", types.DefaultUserServer), Sender: mallory,
		}, ID: "M2", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")}}},
	}
	c.handleMessage(t.Context(), sink, dev, media, forgedRevoke)

	if sink.Has("deleted 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want the forged revoke dropped", sink.Lines())
	}
}

// TestHandleMessage_RefusesAnEditFromSomeoneElse covers Vuln 1's other
// half: Mallory sends an edit for Nadia's message M0. Without
// authorizedActor's check, the stored message would be rewritten under
// Nadia's name with whatever text Mallory chose.
func TestHandleMessage_RefusesAnEditFromSomeoneElse(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	seedMessageKey(t, media, "15551234567@s.whatsapp.net", "M0", messageKey{senderID: "15551234567@s.whatsapp.net", fromMe: false})

	mallory := types.NewJID("15559876543", types.DefaultUserServer)
	forgedEdit := &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{
			Chat: types.NewJID("15551234567", types.DefaultUserServer), Sender: mallory,
		}, ID: "M2", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")},
			EditedMessage: &waE2E.Message{Conversation: strPtr("New bank details: https://evil.example/pay")},
		}},
	}
	c.handleMessage(t.Context(), sink, dev, media, forgedEdit)

	if sink.Has("edited 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want the forged edit dropped", sink.Lines())
	}
}

// TestHandleMessage_RefusesARevokeOfAnUnknownMessage covers an id this
// connector never saved a key for: nothing recorded to check the
// revoke against, so it is dropped rather than applied on trust.
func TestHandleMessage_RefusesARevokeOfAnUnknownMessage(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)

	revokeMsg := &events.Message{
		Info:    liveInfo(),
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("never-seen")}}},
	}
	c.handleMessage(t.Context(), sink, dev, media, revokeMsg)

	if sink.Has("deleted 15551234567@s.whatsapp.net never-seen") {
		t.Errorf("events = %q, want the revoke of an unknown message dropped", sink.Lines())
	}
}

// TestHandleMessage_RevokeSurvivesAnIDReusedByADifferentSender covers
// the full chain behind the remaining gap in Vuln 1's fix: Alice sends
// M0 through handleMessage - not seeded directly, so her key is saved
// the same way a live message's always is - then Mallory sends her own
// message naming the same id before revoking it. Her message must
// never be allowed to overwrite message_keys' record of Alice as M0's
// sender (see keys.go's putMessageKey), or her own forged revoke would
// pass authorizedActor and delete Alice's message under Alice's name.
func TestHandleMessage_RevokeSurvivesAnIDReusedByADifferentSender(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	chat := types.NewJID("15551234567", types.DefaultUserServer)
	alice := types.NewJID("15551111111", types.DefaultUserServer)
	mallory := types.NewJID("15559876543", types.DefaultUserServer)

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: alice}, ID: "M0", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hello from Alice")},
	})
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M0", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{Conversation: strPtr("Mallory reusing Alice's id")},
	})

	forgedRevoke := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M2", Timestamp: time.Unix(3, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")}}},
	}
	c.handleMessage(t.Context(), sink, dev, media, forgedRevoke)

	if sink.Has("deleted " + remoteID(chat) + " M0") {
		t.Errorf("events = %q, want Alice's message to survive Mallory's id-reuse revoke", sink.Lines())
	}
}

// TestHandleMessage_RevokeSurvivesAnIDReusedForOurOwnSentMessage covers
// the same chain for a message this account sent: Mallory naming the
// same id as a message this account just sent must not let her revoke
// it on our behalf by overwriting message_keys' from-me record.
func TestHandleMessage_RevokeSurvivesAnIDReusedForOurOwnSentMessage(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	chat := types.NewJID("15551234567", types.DefaultUserServer)
	mallory := types.NewJID("15559876543", types.DefaultUserServer)
	seedMessageKey(t, media, remoteID(chat), "M0", messageKey{fromMe: true})

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M0", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{Conversation: strPtr("Mallory reusing our own sent id")},
	})

	forgedRevoke := &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M2", Timestamp: time.Unix(3, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")}}},
	}
	c.handleMessage(t.Context(), sink, dev, media, forgedRevoke)

	if sink.Has("deleted " + remoteID(chat) + " M0") {
		t.Errorf("events = %q, want our own sent message to survive Mallory's id-reuse revoke", sink.Lines())
	}
}

// TestHandleMessage_AllowsOurOwnRevokeFromAnotherDevice covers fromMe:
// this account revoking its own message from another linked device
// must still work, even though the account's own messages are saved
// with no sender id of their own (see keys.go's senderKeyID).
func TestHandleMessage_AllowsOurOwnRevokeFromAnotherDevice(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	seedMessageKey(t, media, "15551234567@s.whatsapp.net", "M0", messageKey{fromMe: true})

	ownRevoke := &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{
			Chat: types.NewJID("15551234567", types.DefaultUserServer), IsFromMe: true,
		}, ID: "M2", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M0")}}},
	}
	c.handleMessage(t.Context(), sink, dev, media, ownRevoke)

	if !sink.Has("deleted 15551234567@s.whatsapp.net M0") {
		t.Errorf("events = %q, want our own revoke from another device reported", sink.Lines())
	}
}
