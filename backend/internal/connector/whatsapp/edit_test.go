package whatsapp

// EditMessage is driven through a fake device, the way delete_test.go
// drives DeleteMessages, since the fake is the only way to see what was
// sent without reaching WhatsApp's servers.

import (
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestEditMessage_EditsOurOwnMessage(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: "", fromMe: true, timestamp: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}

	if err := c.EditMessage(t.Context(), directChat, "M1", "hi there", nil); err != nil {
		t.Fatal(err)
	}

	if len(dev.sent) != 1 {
		t.Fatalf("sent = %+v, want one edit message", dev.sent)
	}
	pm := dev.sent[0].msg.GetEditedMessage().GetMessage().GetProtocolMessage()
	if pm.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT || pm.GetKey().GetID() != "M1" || !pm.GetKey().GetFromMe() {
		t.Errorf("protocol message = %+v, want an edit of M1, from us", pm)
	}
	if got := pm.GetEditedMessage().GetConversation(); got != "hi there" {
		t.Errorf("edited text = %q, want %q", got, "hi there")
	}
}

func TestEditMessage_RefusesSomeoneElsesMessage(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: remoteID(directPeer), fromMe: false, timestamp: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}

	err := c.EditMessage(t.Context(), directChat, "M1", "hi there", nil)
	if !errors.Is(err, connector.ErrEditUnsupported) {
		t.Errorf("EditMessage(someone else's message) = %v, want ErrEditUnsupported", err)
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent for a refused edit", dev.sent)
	}
}

func TestEditMessage_RefusesAnUnknownMessage(t *testing.T) {
	t.Parallel()

	_, _, c, _ := connectedMediaFixture(t)

	err := c.EditMessage(t.Context(), directChat, "never-seen", "hi there", nil)
	if !errors.Is(err, connector.ErrEditUnsupported) {
		t.Errorf("EditMessage(unknown message) = %v, want ErrEditUnsupported", err)
	}
}

func TestEditMessage_RefusesPastTheEditWindow(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	old := time.Now().Add(-whatsmeow.EditWindow - time.Minute).UnixMilli()

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: "", fromMe: true, timestamp: old}); err != nil {
		t.Fatal(err)
	}

	err := c.EditMessage(t.Context(), directChat, "M1", "hi there", nil)
	if !errors.Is(err, connector.ErrEditUnsupported) {
		t.Errorf("EditMessage(expired) = %v, want ErrEditUnsupported", err)
	}
	if len(dev.sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent past the edit window", dev.sent)
	}
}

func TestEditMessage_ReappliesMentions(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: "", fromMe: true, timestamp: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}

	text := "hi @Ben"
	mentions := []domain.Mention{{UserID: remoteID(directPeer), Name: "Ben", Offset: 3, Length: 4}}
	if err := c.EditMessage(t.Context(), directChat, "M1", text, mentions); err != nil {
		t.Fatal(err)
	}

	pm := dev.sent[0].msg.GetEditedMessage().GetMessage().GetProtocolMessage()
	ext := pm.GetEditedMessage().GetExtendedTextMessage()
	if ext.GetContextInfo().GetMentionedJID() == nil {
		t.Errorf("edited message = %+v, want MentionedJID carried over", ext)
	}
}

func TestEditMessage_FailsForABadConversationID(t *testing.T) {
	t.Parallel()

	_, _, c, _ := connectedMediaFixture(t)

	err := c.EditMessage(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}, "M1", "hi there", nil)
	if !errors.Is(err, errBadRemoteID) {
		t.Errorf("EditMessage = %v, want errBadRemoteID", err)
	}
}

func TestEditMessage_WrapsAWhatsAppError(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	if err := media.putMessageKey(t.Context(), directChat.RemoteID, "M1", messageKey{senderID: "", fromMe: true, timestamp: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	dev.sendErr = errors.New("server unavailable")

	if err := c.EditMessage(t.Context(), directChat, "M1", "hi there", nil); !errors.Is(err, dev.sendErr) {
		t.Errorf("EditMessage = %v, want it to wrap the device's error", err)
	}
}
