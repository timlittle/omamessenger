package telegram

import (
	"errors"
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// chatWithNadia is a direct conversation the tests send to.
var chatWithNadia = domain.Conversation{AccountID: "tg", RemoteID: "user:42:99"}

func TestSend_ReportsSentThenReadWhenTheyReadIt(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesSendMessageRequest{}, &tg.UpdateShortSentMessage{ID: 77})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)
	c.learn(chatWithNadia.RemoteID)
	if err := c.Send(t.Context(), chatWithNadia, domain.Message{ID: "m1", Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	c.readUpTo(t.Context(), &sink, "user:42", 77)

	want := []string{"outgoing m1 77 " + domain.StatusSent, "outgoing m1  " + domain.StatusRead}
	if got := sink.Lines(); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}

	req, ok := f.sent()[0].(*tg.MessagesSendMessageRequest)
	if !ok || req.Message != "hi" || req.RandomID == 0 {
		t.Errorf("request = %+v, want the text with a random id", f.sent()[0])
	}
}

func TestSend_Fails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		c    func() *Connector
		conv domain.Conversation
	}{
		{"before signing in", func() *Connector { return New(domain.Account{ID: "tg"}, "") }, chatWithNadia},
		{"to a malformed peer", func() *Connector { return connectedTo(newFakeTelegram(), &connectortest.Sink{}) }, domain.Conversation{RemoteID: "bad"}},
		{"when Telegram refuses", func() *Connector { return connectedTo(newFakeTelegram(), &connectortest.Sink{}) }, chatWithNadia},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.c().Send(t.Context(), tt.conv, domain.Message{ID: "m1", Text: "hi"}); err == nil {
				t.Error("Send succeeded, want an error")
			}
		})
	}
}

func TestMarkRead_UsesTheChannelCallForChannels(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesReadHistoryRequest{}, &tg.MessagesAffectedMessages{})
	f.reply(&tg.ChannelsReadHistoryRequest{}, &tg.BoolTrue{})

	c := connectedTo(f, &connectortest.Sink{})
	for _, remote := range []string{"user:42:99", "channel:5:3"} {
		if err := c.MarkRead(t.Context(), domain.Conversation{RemoteID: remote}); err != nil {
			t.Fatal(err)
		}
	}

	sent := f.sent()
	if _, ok := sent[0].(*tg.MessagesReadHistoryRequest); !ok {
		t.Errorf("direct chat read with %T", sent[0])
	}

	if _, ok := sent[1].(*tg.ChannelsReadHistoryRequest); !ok {
		t.Errorf("channel read with %T", sent[1])
	}
}

func TestMarkRead_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in":    New(domain.Account{ID: "tg"}, "").MarkRead(t.Context(), chatWithNadia),
		"a malformed peer":     connected.MarkRead(t.Context(), domain.Conversation{RemoteID: "bad"}),
		"when Telegram errors": connected.MarkRead(t.Context(), chatWithNadia),
	} {
		if err == nil {
			t.Errorf("MarkRead %s succeeded, want an error", name)
		}
	}
}

func TestSubmitAuth_OnlyWhileSigningIn(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "tg"}, "")
	if err := c.SubmitAuth(t.Context(), "code", "1"); !errors.Is(err, errNotSigningIn) {
		t.Errorf("SubmitAuth before sign-in = %v, want errNotSigningIn", err)
	}

	c.setWaiting(true)
	if err := c.SubmitAuth(t.Context(), "code", "12345"); err != nil {
		t.Fatal(err)
	}

	if a := <-c.answers; a != (answer{step: "code", value: "12345"}) {
		t.Errorf("answer = %+v", a)
	}
}

func TestSentID_FindsTheIDInEitherReply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result tg.UpdatesClass
		want   int
		ok     bool
	}{
		{"short reply", &tg.UpdateShortSentMessage{ID: 5}, 5, true},
		{"full updates", &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateNewMessage{}, &tg.UpdateMessageID{ID: 6}}}, 6, true},
		{"no id", &tg.UpdatesTooLong{}, 0, false},
	}

	for _, tt := range tests {
		if got, ok := sentID(tt.result); got != tt.want || ok != tt.ok {
			t.Errorf("%s: sentID = %d, %t; want %d, %t", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDisconnected_StopsSends(t *testing.T) {
	t.Parallel()

	c := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	c.disconnected()

	if err := c.Send(t.Context(), chatWithNadia, domain.Message{}); !errors.Is(err, errNotConnected) {
		t.Errorf("Send after disconnect = %v, want errNotConnected", err)
	}
}
