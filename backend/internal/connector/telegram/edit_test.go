package telegram

import (
	"errors"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestEditMessage_SendsTheNewTextAndEntities checks that EditMessage
// sends the message's new text and reapplies its mentions as entities.
func TestEditMessage_SendsTheNewTextAndEntities(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesEditMessageRequest{}, &tg.Updates{})

	c := connectedAndKnown(f, &connectortest.Sink{}, chatWithNadia.RemoteID)
	mentions := []domain.Mention{{UserID: "user:1:1", Name: "Ben", Offset: 0, Length: 3}}
	if err := c.EditMessage(t.Context(), chatWithNadia, "7", "Ben, hi", mentions); err != nil {
		t.Fatal(err)
	}

	req, ok := f.sent()[0].(*tg.MessagesEditMessageRequest)
	if !ok || req.ID != 7 || req.Message != "Ben, hi" || len(req.Entities) != 1 {
		t.Errorf("request = %+v, want an edit of 7 to %q with one entity", f.sent()[0], "Ben, hi")
	}
}

func TestEditMessage_MapsARefusalToErrEditUnsupported(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.failNext(&tg.MessagesEditMessageRequest{}, tgerr.New(400, "MESSAGE_EDIT_TIME_EXPIRED"))

	c := connectedAndKnown(f, &connectortest.Sink{}, chatWithNadia.RemoteID)
	err := c.EditMessage(t.Context(), chatWithNadia, "7", "too late", nil)
	if !errors.Is(err, connector.ErrEditUnsupported) {
		t.Errorf("EditMessage refused = %v, want ErrEditUnsupported", err)
	}
}

func TestEditMessage_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in":      New(domain.Account{ID: "tg"}, "").EditMessage(t.Context(), chatWithNadia, "7", "hi", nil),
		"a malformed peer":       connected.EditMessage(t.Context(), domain.Conversation{RemoteID: "bad"}, "7", "hi", nil),
		"a malformed message id": connected.EditMessage(t.Context(), chatWithNadia, "not-a-number", "hi", nil),
	} {
		if err == nil {
			t.Errorf("EditMessage %s succeeded, want an error", name)
		}
	}
}
