package whatsapp

// The handlers in contacts.go are unexported, so these tests call them
// directly over a fake device, the way history_test.go drives
// handleHistorySync and live_test.go drives live.go's handlers.

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestHandleContactUpdate_RetitlesAnAlreadyKnownDirectChat(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	jid := types.NewJID("15551234567", types.DefaultUserServer)
	c.rememberName(remoteID(jid), "+15551234567", nameRankPushName) // the chat's current, weak fallback
	var sink connectortest.Sink

	c.handleContactUpdate(t.Context(), &sink, &events.Contact{
		JID: jid, Action: &waSyncAction.ContactAction{FullName: strPtr("Nadia Rahman")},
	})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia Rahman") {
		t.Errorf("events = %q, want the conversation re-titled with the newly synced contact name", sink.Lines())
	}
}

func TestHandleContactUpdate_IgnoresAnActionThatNamesNoOne(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	jid := types.NewJID("15551234567", types.DefaultUserServer)
	var sink connectortest.Sink

	c.handleContactUpdate(t.Context(), &sink, &events.Contact{JID: jid, Action: &waSyncAction.ContactAction{}})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a contact action with no name", sink.Lines())
	}
}

func TestHandlePushNameUpdate_RetitlesAFallbackTitledChat(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	jid := types.NewJID("15551234567", types.DefaultUserServer)
	var sink connectortest.Sink

	c.handlePushNameUpdate(t.Context(), &sink, &events.PushName{JID: jid, NewPushName: "Nadia"})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
		t.Errorf("events = %q, want the conversation re-titled with the new push name", sink.Lines())
	}
}

func TestHandlePushNameUpdate_NeverDowngradesAResolvedContactName(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	jid := types.NewJID("15551234567", types.DefaultUserServer)
	c.rememberName(remoteID(jid), "Nadia Rahman", nameRankContact)
	var sink connectortest.Sink

	c.handlePushNameUpdate(t.Context(), &sink, &events.PushName{JID: jid, NewPushName: "nads99"})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want a push name never to replace an already-resolved contact name", sink.Lines())
	}
}

func TestHandleAppStateSyncComplete_RechecksEveryKnownDirectChat(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	jid := types.NewJID("987654", types.HiddenUserServer)
	c.noteChat(remoteID(jid), domain.KindDirect)
	c.rememberName(remoteID(jid), "Unknown contact", nameRankPushName)

	dev := newFakeDevice()
	dev.contactNames = map[string]string{"987654@lid": "Priya Nair"}
	var sink connectortest.Sink

	c.handleAppStateSyncComplete(t.Context(), &sink, dev, &events.AppStateSyncComplete{})

	if !sink.Has("conversation 987654@lid Priya Nair") {
		t.Errorf("events = %q, want the LID chat re-titled once its contact resolves", sink.Lines())
	}
}

func TestHandleAppStateSyncComplete_NeverTouchesAGroup(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	group := types.NewJID("12345-1600000000", types.GroupServer)
	c.noteChat(remoteID(group), domain.KindGroup)

	dev := newFakeDevice()
	dev.contactNames = map[string]string{"12345-1600000000@g.us": "should never be used"}
	var sink connectortest.Sink

	c.handleAppStateSyncComplete(t.Context(), &sink, dev, &events.AppStateSyncComplete{})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want the rescan to skip known groups entirely", sink.Lines())
	}
}

func TestContactActionName_PrefersFullNameOverFirstName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		act  *waSyncAction.ContactAction
		want string
	}{
		{"full name wins", &waSyncAction.ContactAction{FullName: strPtr("Nadia Rahman"), FirstName: strPtr("Nadia")}, "Nadia Rahman"},
		{"first name alone", &waSyncAction.ContactAction{FirstName: strPtr("Nadia")}, "Nadia"},
		{"neither", &waSyncAction.ContactAction{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := contactActionName(tt.act); got != tt.want {
				t.Errorf("contactActionName(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
