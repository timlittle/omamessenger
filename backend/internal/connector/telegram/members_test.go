package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMembers checks that Members asks the right Telegram call for each
// kind of conversation and reports no members for a direct chat, which
// Telegram has no participant list for.
func TestMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(f *fakeTelegram)
		remote string
		want   []domain.Member
	}{
		{
			name: "from a channel",
			setup: func(f *fakeTelegram) {
				f.reply(&tg.ChannelsGetParticipantsRequest{}, &tg.ChannelsChannelParticipants{
					Users: []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia", LastName: "Rahman"}},
				})
			},
			remote: "channel:5:3",
			want:   []domain.Member{{ID: "user:42:99", Name: "Nadia Rahman"}},
		},
		{
			name: "from a basic group",
			setup: func(f *fakeTelegram) {
				f.reply(&tg.MessagesGetFullChatRequest{}, &tg.MessagesChatFull{
					FullChat: &tg.ChatFull{ID: 7, Participants: &tg.ChatParticipants{ChatID: 7}},
					Users:    []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia"}},
				})
			},
			remote: "chat:7",
			want:   []domain.Member{{ID: "user:42:99", Name: "Nadia"}},
		},
		{
			name:   "none for a direct chat",
			setup:  func(*fakeTelegram) {},
			remote: "user:42:99",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTelegram()
			tt.setup(f)
			c := connectedTo(f, &connectortest.Sink{})

			got, err := c.Members(t.Context(), domain.Conversation{RemoteID: tt.remote})
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(got, tt.want) {
				t.Errorf("members = %+v, want %+v", got, tt.want)
			}
		})
	}
}
