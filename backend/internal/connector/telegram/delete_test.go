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

// TestDeleteMessages_UsesTheChatCall checks that a direct chat's delete
// goes through the plain message call, with Revoke following forMe.
func TestDeleteMessages_UsesTheChatCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ids    []string
		revoke bool
	}{
		{"with revoke", []string{"7", "8"}, true},
		{"for me only, which does not revoke", []string{"7"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTelegram()
			f.reply(&tg.MessagesDeleteMessagesRequest{}, &tg.MessagesAffectedMessages{})

			c := connectedAndKnown(f, &connectortest.Sink{}, chatWithNadia.RemoteID)
			if err := c.DeleteMessages(t.Context(), chatWithNadia, tt.ids, tt.revoke); err != nil {
				t.Fatal(err)
			}

			req, ok := f.sent()[0].(*tg.MessagesDeleteMessagesRequest)
			if !ok || req.Revoke != tt.revoke || len(req.ID) != len(tt.ids) {
				t.Errorf("request = %+v, want revoke %t of %v", f.sent()[0], tt.revoke, tt.ids)
			}
		})
	}
}

func TestDeleteMessages_UsesTheChannelCallForAChannel(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ChannelsDeleteMessagesRequest{}, &tg.MessagesAffectedMessages{})

	c := connectedTo(f, &connectortest.Sink{})
	if err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: "channel:5:3"}, []string{"9"}, true); err != nil {
		t.Fatal(err)
	}

	req, ok := f.sent()[0].(*tg.ChannelsDeleteMessagesRequest)
	channel, chOK := req.Channel.(*tg.InputChannel)
	if !ok || !chOK || channel.ChannelID != 5 || len(req.ID) != 1 || req.ID[0] != 9 {
		t.Errorf("request = %+v, want a channel delete of [9]", f.sent()[0])
	}
}

func TestDeleteMessages_RefusesForMeInAChannel(t *testing.T) {
	t.Parallel()

	c := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	err := c.DeleteMessages(t.Context(), domain.Conversation{RemoteID: "channel:5:3"}, []string{"9"}, false)
	if !errors.Is(err, connector.ErrDeleteUnsupported) {
		t.Errorf("DeleteMessages(forMe) in a channel = %v, want ErrDeleteUnsupported", err)
	}
}

func TestDeleteMessages_MapsARefusalToErrDeleteUnsupported(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.failNext(&tg.MessagesDeleteMessagesRequest{}, tgerr.New(400, "MESSAGE_DELETE_FORBIDDEN"))

	c := connectedAndKnown(f, &connectortest.Sink{}, chatWithNadia.RemoteID)
	err := c.DeleteMessages(t.Context(), chatWithNadia, []string{"7"}, true)
	if !errors.Is(err, connector.ErrDeleteUnsupported) {
		t.Errorf("DeleteMessages refused = %v, want ErrDeleteUnsupported", err)
	}
}

func TestDeleteMessages_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in":      New(domain.Account{ID: "tg"}, "").DeleteMessages(t.Context(), chatWithNadia, []string{"7"}, true),
		"a malformed peer":       connected.DeleteMessages(t.Context(), domain.Conversation{RemoteID: "bad"}, []string{"7"}, true),
		"a malformed message id": connected.DeleteMessages(t.Context(), chatWithNadia, []string{"not-a-number"}, true),
	} {
		if err == nil {
			t.Errorf("DeleteMessages %s succeeded, want an error", name)
		}
	}
}
