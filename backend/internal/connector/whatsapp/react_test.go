package whatsapp

// React is driven through a fake device, the way send_test.go drives
// Send, since the fake is the only way to see the reaction message
// Connector asked whatsmeow to send without reaching WhatsApp's
// servers.

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestReact_SendsAndTalliesADirectReaction(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.React(t.Context(), directChat, "M1", "👍"); err != nil {
		t.Fatal(err)
	}

	if len(dev.sent) != 1 {
		t.Fatalf("sent = %+v, want one reaction message", dev.sent)
	}

	r := dev.sent[0].msg.GetReactionMessage()
	if r.GetText() != "👍" || r.GetKey().GetID() != "M1" || r.GetKey().GetFromMe() {
		t.Errorf("reaction = %+v, want it to react to M1, not sent by us", r)
	}
	if r.GetKey().GetParticipant() != "" {
		t.Errorf("participant = %q, want none in a direct chat", r.GetKey().GetParticipant())
	}

	if !sink.Has("reacted " + directChat.RemoteID + " M1 1") {
		t.Errorf("events = %q, want one reaction chip reported", sink.Lines())
	}
}

func TestReact_SetsTheParticipantInAGroup(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	sender := types.NewJID("15557654321", types.DefaultUserServer)
	if err := media.putMessageKey(t.Context(), groupChat.RemoteID, "M2", messageKey{senderID: remoteID(sender), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.React(t.Context(), groupChat, "M2", "❤️"); err != nil {
		t.Fatal(err)
	}

	r := dev.sent[0].msg.GetReactionMessage()
	if r.GetKey().GetParticipant() != remoteID(sender) {
		t.Errorf("participant = %q, want the group member who sent M2", r.GetKey().GetParticipant())
	}
}

func TestReact_ToOurOwnMessageSetsNoParticipant(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	if err := media.putMessageKey(t.Context(), groupChat.RemoteID, "M3", messageKey{senderID: "", fromMe: true, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.React(t.Context(), groupChat, "M3", "👍"); err != nil {
		t.Fatal(err)
	}

	r := dev.sent[0].msg.GetReactionMessage()
	if !r.GetKey().GetFromMe() || r.GetKey().GetParticipant() != "" {
		t.Errorf("key = %+v, want fromMe with no participant for our own message", r.GetKey())
	}
}

func TestReact_ClearingSendsAnEmptyReactionAndLowersTheTally(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.React(t.Context(), directChat, "M1", "👍"); err != nil {
		t.Fatal(err)
	}
	if err := c.React(t.Context(), directChat, "M1", ""); err != nil {
		t.Fatal(err)
	}

	r := dev.sent[1].msg.GetReactionMessage()
	if r.GetText() != "" {
		t.Errorf("text = %q, want an empty reaction to clear it", r.GetText())
	}
	if !sink.Has("reacted " + directChat.RemoteID + " M1 0") {
		t.Errorf("events = %q, want the chip gone after clearing", sink.Lines())
	}
}

func TestReact_UnknownTargetFailsSafely(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	err := c.React(t.Context(), directChat, "never-seen", "👍")
	if !errors.Is(err, errUnknownReactionTarget) {
		t.Errorf("React = %v, want errUnknownReactionTarget", err)
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent for an unknown target", dev.sent)
	}
}

func TestReact_FailsWithoutAMediaStore(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.React(t.Context(), directChat, "M1", "👍"); !errors.Is(err, errNotConnected) {
		t.Errorf("React = %v, want errNotConnected without a media store", err)
	}
}

func TestReact_WrapsAWhatsAppError(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.sendErr = errors.New("server unavailable")
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	media := c.mediaFor()

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: 1000}); err != nil {
		t.Fatal(err)
	}

	if err := c.React(t.Context(), directChat, "M1", "👍"); !errors.Is(err, dev.sendErr) {
		t.Errorf("React = %v, want it to wrap the device's error", err)
	}
	if sink.Has("reacted " + directChat.RemoteID + " M1 1") {
		t.Error("a reaction was tallied despite the send failing")
	}
}

func TestReact_FailsForABadConversationID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)

	err := c.React(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}, "M1", "👍")
	if !errors.Is(err, errBadRemoteID) {
		t.Errorf("React = %v, want errBadRemoteID", err)
	}
}
