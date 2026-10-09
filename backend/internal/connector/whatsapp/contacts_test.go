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

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)
	c.rememberName(remoteID(jid), "+15551234567", nameRankPushName) // the chat's current, weak fallback

	c.handleContactUpdate(t.Context(), sink, dev, nil, &events.Contact{
		JID: jid, Action: &waSyncAction.ContactAction{FullName: strPtr("Nadia Rahman")},
	})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia Rahman") {
		t.Errorf("events = %q, want the conversation re-titled with the newly synced contact name", sink.Lines())
	}
}

func TestHandleContactUpdate_IgnoresAnActionThatNamesNoOne(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handleContactUpdate(t.Context(), sink, dev, nil, &events.Contact{JID: jid, Action: &waSyncAction.ContactAction{}})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a contact action with no name", sink.Lines())
	}
}

func TestHandleContactUpdate_NeverRetitlesTheSelfChat(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	own := types.NewJID("15551234567", types.DefaultUserServer)
	dev.selfJID = own

	c.handleContactUpdate(t.Context(), sink, dev, nil, &events.Contact{
		JID: own, Action: &waSyncAction.ContactAction{FullName: strPtr("Tim Little")},
	})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want the self-chat left untouched rather than re-titled with a resolved contact name", sink.Lines())
	}
}

// TestHandleContactUpdate_CorrectsTheSendersNameOnAlreadyStoredMessages
// confirms a resolved contact name also reaches Sink.SenderName (see
// connector.SenderNamer), which is what lets a group's preview catch
// up when one of its members is who just resolved, not only this
// person's own direct chat title.
func TestHandleContactUpdate_CorrectsTheSendersNameOnAlreadyStoredMessages(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	jid := types.NewJID("15551234567", types.DefaultUserServer)
	dev := newFakeDevice()
	sink := &recordingSink{Sink: &connectortest.Sink{}}

	c.handleContactUpdate(t.Context(), sink, dev, nil, &events.Contact{
		JID: jid, Action: &waSyncAction.ContactAction{FullName: strPtr("Nadia Rahman")},
	})

	want := senderNameCall{accountID: "wa", senderRemoteID: "15551234567@s.whatsapp.net", name: "Nadia Rahman"}
	if len(sink.senderNames) != 1 || sink.senderNames[0] != want {
		t.Errorf("SenderName calls = %+v, want exactly [%+v]", sink.senderNames, want)
	}
}

// TestHandleContactUpdate_RetitlesAPNKnownChatEvenWhenNamedByItsLID
// confirms a contact update does not create a ghost conversation when
// WhatsApp names the update's subject by their LID while this
// connector already knows their chat by its phone JID: the fix for
// retitleDirectChat resolving jid through chatID rather than the bare
// remote id.
func TestHandleContactUpdate_RetitlesAPNKnownChatEvenWhenNamedByItsLID(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)

	c, dev, sink := handlerFixture(t)
	c.rememberName(remoteID(phone), "+15551234567", nameRankPushName) // the chat's current, weak fallback, keyed by phone JID
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	c.handleContactUpdate(t.Context(), sink, dev, nil, &events.Contact{
		JID: lid, Action: &waSyncAction.ContactAction{FullName: strPtr("Nadia Rahman")},
	})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia Rahman") {
		t.Errorf("events = %q, want the already-known phone-JID chat retitled, not a new LID-keyed one", sink.Lines())
	}
}

// TestHandlePushNameUpdate_RetitlesAPNKnownChatEvenWhenNamedByItsLID
// is handleContactUpdate's same regression for a push name update.
func TestHandlePushNameUpdate_RetitlesAPNKnownChatEvenWhenNamedByItsLID(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)

	c, dev, sink := handlerFixture(t)
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	c.handlePushNameUpdate(t.Context(), sink, dev, nil, &events.PushName{JID: lid, NewPushName: "Nadia"})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
		t.Errorf("events = %q, want the already-known phone-JID chat retitled, not a new LID-keyed one", sink.Lines())
	}
}

// TestHandleBusinessNameUpdate_RetitlesAFallbackTitledChat confirms a
// business's first verified name resolved after their chat already
// exists retitles it.
func TestHandleBusinessNameUpdate_RetitlesAFallbackTitledChat(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handleBusinessNameUpdate(t.Context(), sink, dev, nil, &events.BusinessName{JID: jid, NewBusinessName: "Acme Support"})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Acme Support") {
		t.Errorf("events = %q, want the conversation re-titled with the new business name", sink.Lines())
	}
}

// TestHandleBusinessNameUpdate_RetitlesAPNKnownChatEvenWhenNamedByItsLID
// is handleContactUpdate's same regression for a verified business
// name update.
func TestHandleBusinessNameUpdate_RetitlesAPNKnownChatEvenWhenNamedByItsLID(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)

	c, dev, sink := handlerFixture(t)
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	c.handleBusinessNameUpdate(t.Context(), sink, dev, nil, &events.BusinessName{JID: lid, NewBusinessName: "Acme Support"})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Acme Support") {
		t.Errorf("events = %q, want the already-known phone-JID chat retitled, not a new LID-keyed one", sink.Lines())
	}
}

func TestHandlePushNameUpdate_RetitlesAFallbackTitledChat(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handlePushNameUpdate(t.Context(), sink, dev, nil, &events.PushName{JID: jid, NewPushName: "Nadia"})

	if !sink.Has("conversation 15551234567@s.whatsapp.net Nadia") {
		t.Errorf("events = %q, want the conversation re-titled with the new push name", sink.Lines())
	}
}

func TestHandlePushNameUpdate_NeverDowngradesAResolvedContactName(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)
	c.rememberName(remoteID(jid), "Nadia Rahman", nameRankContact)

	c.handlePushNameUpdate(t.Context(), sink, dev, nil, &events.PushName{JID: jid, NewPushName: "nads99"})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want a push name never to replace an already-resolved contact name", sink.Lines())
	}
}

// TestHandlePushNameUpdate_IgnoresAnEmptyPushName is handleContactUpdate's
// same regression (TestHandleContactUpdate_IgnoresAnActionThatNamesNoOne)
// for a push name update: WhatsApp can report one with no name set.
func TestHandlePushNameUpdate_IgnoresAnEmptyPushName(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handlePushNameUpdate(t.Context(), sink, dev, nil, &events.PushName{JID: jid, NewPushName: ""})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a push name update with no name", sink.Lines())
	}
}

// TestHandleBusinessNameUpdate_IgnoresAnEmptyBusinessName is the same
// regression for a verified business name update.
func TestHandleBusinessNameUpdate_IgnoresAnEmptyBusinessName(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handleBusinessNameUpdate(t.Context(), sink, dev, nil, &events.BusinessName{JID: jid, NewBusinessName: ""})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a business name update with no name", sink.Lines())
	}
}

func TestHandleAppStateSyncComplete_NeverRetitlesTheSelfChat(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	own := types.NewJID("15551234567", types.DefaultUserServer)
	c.noteChat(remoteID(own), domain.KindDirect)

	dev.selfJID = own
	dev.contactNames = map[string]string{"15551234567@s.whatsapp.net": "Tim Little"}

	c.handleAppStateSyncComplete(t.Context(), sink, dev, nil, &events.AppStateSyncComplete{})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want the self-chat's rescan to report nothing rather than a resolved contact name", sink.Lines())
	}
}

func TestHandleAppStateSyncComplete_RechecksEveryKnownDirectChat(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("987654", types.HiddenUserServer)
	c.noteChat(remoteID(jid), domain.KindDirect)
	c.rememberName(remoteID(jid), "Unknown contact", nameRankPushName)

	dev.contactNames = map[string]string{"987654@lid": "Priya Nair"}

	c.handleAppStateSyncComplete(t.Context(), sink, dev, nil, &events.AppStateSyncComplete{})

	if !sink.Has("conversation 987654@lid Priya Nair") {
		t.Errorf("events = %q, want the LID chat re-titled once its contact resolves", sink.Lines())
	}
}

func TestHandleAppStateSyncComplete_NeverTouchesAGroup(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	group := types.NewJID("12345-1600000000", types.GroupServer)
	c.noteChat(remoteID(group), domain.KindGroup)

	dev.contactNames = map[string]string{"12345-1600000000@g.us": "should never be used"}

	c.handleAppStateSyncComplete(t.Context(), sink, dev, nil, &events.AppStateSyncComplete{})

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
