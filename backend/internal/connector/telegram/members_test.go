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
					Users: []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia", LastName: "Rahman", Contact: true}},
				})
			},
			remote: "channel:5:3",
			want:   []domain.Member{{ID: "user:42:99", Name: "Nadia Rahman", Verified: true}},
		},
		{
			name: "from a basic group",
			setup: func(f *fakeTelegram) {
				f.reply(&tg.MessagesGetFullChatRequest{}, &tg.MessagesChatFull{
					FullChat: &tg.ChatFull{ID: 7, Participants: &tg.ChatParticipants{ChatID: 7}},
					Users:    []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia", Contact: true}},
				})
			},
			remote: "chat:7",
			want:   []domain.Member{{ID: "user:42:99", Name: "Nadia", Verified: true}},
		},
		{
			// A group member's own profile name is just as able to carry
			// a self-chosen claim as a direct chat's sender is, unless
			// Telegram says this account has saved them as a contact or
			// Telegram itself verifies them (see
			// domain.Message.SenderVerified and docs/decisions.md).
			name: "a bare profile name with no contact or verified badge is unverified",
			setup: func(f *fakeTelegram) {
				f.reply(&tg.MessagesGetFullChatRequest{}, &tg.MessagesChatFull{
					FullChat: &tg.ChatFull{ID: 8, Participants: &tg.ChatParticipants{ChatID: 8}},
					Users:    []tg.UserClass{&tg.User{ID: 43, AccessHash: 100, FirstName: "Mum"}},
				})
			},
			remote: "chat:8",
			want:   []domain.Member{{ID: "user:43:100", Name: "Mum", Verified: false}},
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
