package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestRefreshMessages_ReportsEachMessageThroughHistory(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{
			&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaPhoto{Photo: photo}},
			&tg.Message{ID: 41, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi"},
		},
	})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	if err := c.RefreshMessages(t.Context(), chatWithNadia, []string{"40", "41"}); err != nil {
		t.Fatal(err)
	}

	want := []string{"history user:42:99 40", "history user:42:99 41"}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}

	req, ok := f.sent()[0].(*tg.MessagesGetMessagesRequest)
	if !ok || len(req.ID) != 2 {
		t.Fatalf("request = %+v, want 2 ids", f.sent()[0])
	}

	got := sink.Messages()["user:42:99"]
	if len(got) != 2 || got[0].Media == nil || got[0].Media.Kind != domain.MediaPhoto {
		t.Errorf("messages = %+v, want the photo filled in", got)
	}
}

func TestRefreshMessages_FetchesFromAChannel(t *testing.T) {
	t.Parallel()

	conv := domain.Conversation{AccountID: "tg", RemoteID: "channel:5:3"}
	f := newFakeTelegram()
	f.reply(&tg.ChannelsGetMessagesRequest{}, &tg.MessagesMessages{
		Messages: []tg.MessageClass{&tg.Message{ID: 7, PeerID: &tg.PeerChannel{ChannelID: 5}, Message: "hi"}},
	})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	if err := c.RefreshMessages(t.Context(), conv, []string{"7"}); err != nil {
		t.Fatal(err)
	}

	req, ok := f.sent()[0].(*tg.ChannelsGetMessagesRequest)
	if !ok || len(req.ID) != 1 {
		t.Fatalf("request = %+v, want the channel request", f.sent()[0])
	}

	if want := []string{"history channel:5:3 7"}; !slices.Equal(sink.Lines(), want) {
		t.Errorf("events = %q, want %q", sink.Lines(), want)
	}
}

func TestRefreshMessages_BatchesOverOneHundredIDs(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{})

	ids := make([]string, 150)
	for i := range ids {
		ids[i] = "1"
	}

	c := connectedTo(f, &connectortest.Sink{})
	if err := c.RefreshMessages(t.Context(), chatWithNadia, ids); err != nil {
		t.Fatal(err)
	}

	sent := f.sent()
	if len(sent) != 2 {
		t.Fatalf("requests sent = %d, want 2 batches", len(sent))
	}

	first, ok := sent[0].(*tg.MessagesGetMessagesRequest)
	second, ok2 := sent[1].(*tg.MessagesGetMessagesRequest)
	if !ok || !ok2 || len(first.ID) != 100 || len(second.ID) != 50 {
		t.Errorf("batches = %v, %v, want 100 then 50", sent[0], sent[1])
	}
}

func TestRefreshMessages_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in":     New(domain.Account{ID: "tg"}, "").RefreshMessages(t.Context(), chatWithNadia, []string{"40"}),
		"for a malformed id":    connected.RefreshMessages(t.Context(), chatWithNadia, []string{"x"}),
		"for a malformed peer":  connected.RefreshMessages(t.Context(), domain.Conversation{RemoteID: "bad"}, []string{"40"}),
		"when Telegram refuses": connected.RefreshMessages(t.Context(), chatWithNadia, []string{"40"}),
	} {
		if err == nil {
			t.Errorf("RefreshMessages %s succeeded, want an error", name)
		}
	}
}
