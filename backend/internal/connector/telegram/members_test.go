package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMembers_FromAChannel(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ChannelsGetParticipantsRequest{}, &tg.ChannelsChannelParticipants{
		Users: []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia", LastName: "Rahman"}},
	})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)

	got, err := c.Members(t.Context(), domain.Conversation{RemoteID: "channel:5:3"})
	if err != nil {
		t.Fatal(err)
	}

	want := []domain.Member{{ID: "user:42:99", Name: "Nadia Rahman"}}
	if !slices.Equal(got, want) {
		t.Errorf("members = %+v, want %+v", got, want)
	}
}

func TestMembers_FromABasicGroup(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetFullChatRequest{}, &tg.MessagesChatFull{
		FullChat: &tg.ChatFull{ID: 7, Participants: &tg.ChatParticipants{ChatID: 7}},
		Users:    []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia"}},
	})

	var sink connectortest.Sink
	c := connectedTo(f, &sink)

	got, err := c.Members(t.Context(), domain.Conversation{RemoteID: "chat:7"})
	if err != nil {
		t.Fatal(err)
	}

	want := []domain.Member{{ID: "user:42:99", Name: "Nadia"}}
	if !slices.Equal(got, want) {
		t.Errorf("members = %+v, want %+v", got, want)
	}
}

func TestMembers_NoneForADirectChat(t *testing.T) {
	t.Parallel()

	var sink connectortest.Sink
	c := connectedTo(newFakeTelegram(), &sink)

	got, err := c.Members(t.Context(), domain.Conversation{RemoteID: "user:42:99"})
	if err != nil || got != nil {
		t.Errorf("members, err = %+v, %v; want nil, nil", got, err)
	}
}
