package whatsapp

// handleHistorySync is unexported, with no way to drive a real one of
// these blobs without reaching WhatsApp, so these tests call it
// directly, as the Telegram connector's sync tests do for its own
// unexported sync step.

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// historyMsg builds one history sync message with text, FromMe and a
// timestamp of 1.
func historyMsg(id, text string, fromMe bool) *waHistorySync.HistorySyncMsg {
	return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr(id), FromMe: boolPtr(fromMe)},
		Message:          &waE2E.Message{Conversation: strPtr(text)},
		MessageTimestamp: u64(1),
	}}
}

func TestHandleHistorySync_ReportsContactsConversationsAndMessages(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Pushnames: []*waHistorySync.Pushname{{ID: strPtr("15551234567@s.whatsapp.net"), Pushname: strPtr("Nadia")}},
		Conversations: []*waHistorySync.Conversation{
			{
				ID: strPtr("15551234567@s.whatsapp.net"), UnreadCount: u32(1), Pinned: u32(1),
				Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
			},
			{
				ID: strPtr("12345-1600000000@g.us"), DisplayName: strPtr("Climbing Crew"), Archived: boolPtr(true),
				Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H2", "see you there", false)},
			},
		},
	}}

	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	want := []string{
		"contact 15551234567@s.whatsapp.net Nadia",
		"conversation 15551234567@s.whatsapp.net Nadia",
		"organized 15551234567@s.whatsapp.net true false",
		"history 15551234567@s.whatsapp.net H1",
		"unread 15551234567@s.whatsapp.net 1",
		"conversation 12345-1600000000@g.us Climbing Crew",
		"organized 12345-1600000000@g.us false true",
		"history 12345-1600000000@g.us H2",
		"unread 12345-1600000000@g.us 0",
	}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events =\n%q\nwant\n%q", got, want)
	}
}

func TestHandleHistorySync_ResolvesAGroupNameAndCachesIt(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	dev.groupNames = map[string]string{"12345-1600000000@g.us": "Climbing Crew"}
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	conv := func() *waHistorySync.Conversation {
		return &waHistorySync.Conversation{ID: strPtr("12345-1600000000@g.us")}
	}
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{conv()}}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if !sink.Has("conversation 12345-1600000000@g.us Climbing Crew") {
		t.Errorf("events = %q, want the resolved group name", sink.Lines())
	}

	// A second sync of the same group must not ask WhatsApp again.
	e2 := &events.HistorySync{Data: &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{conv()}}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e2)

	if len(dev.groupCalls) != 1 {
		t.Errorf("group name was requested %d times, want 1 (cached after)", len(dev.groupCalls))
	}
}

func TestHandleHistorySync_FallsBackToAGenericNameWhenTheGroupCannotBeResolved(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	dev.groupErr = errors.New("unavailable")
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{ID: strPtr("12345-1600000000@g.us")}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if !sink.Has("conversation 12345-1600000000@g.us Group") {
		t.Errorf("events = %q, want a generic group title", sink.Lines())
	}
}

func TestHandleHistorySync_FallsBackToAPhoneNumberForAnUnnamedDirectChat(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{ID: strPtr("15551234567@s.whatsapp.net")}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if !sink.Has("conversation 15551234567@s.whatsapp.net +15551234567") {
		t.Errorf("events = %q, want the phone number as a fallback title", sink.Lines())
	}
}

func TestHandleHistorySync_PersistsAMessagesMediaReference(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	hm := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key: &waCommon.MessageKey{ID: strPtr("H1")},
		Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			DirectPath: strPtr("/v/abc"), MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3},
			FileLength: u64(99), Mimetype: strPtr("image/jpeg"),
		}},
		MessageTimestamp: u64(1),
	}}
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia"), Messages: []*waHistorySync.HistorySyncMsg{hm}}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	ref, ok, err := media.get(t.Context(), "15551234567@s.whatsapp.net", "H1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("media reference was not saved")
	}
	want := mediaRef{DirectPath: "/v/abc", MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3}, FileLength: 99, Mimetype: "image/jpeg"}
	if !reflect.DeepEqual(ref, want) {
		t.Errorf("media reference = %+v, want %+v", ref, want)
	}
}

func TestHandleHistorySync_DropsAReactionFromHistory(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	hm := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr("H1")},
		Message:          &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Key: &waCommon.MessageKey{ID: strPtr("H0")}, Text: strPtr("👍")}},
		MessageTimestamp: u64(1),
	}}
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia"), Messages: []*waHistorySync.HistorySyncMsg{hm}}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if sink.Has("history 15551234567@s.whatsapp.net H1") {
		t.Error("a reaction protocol message was reported as history")
	}
}

func TestHandleHistorySync_DropsAConversationWithAnUnparseableID(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink

	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{Name: strPtr("Nameless")}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want none for an unparseable id", sink.Lines())
	}
}
