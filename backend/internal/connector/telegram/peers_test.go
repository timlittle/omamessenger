package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestRemoteID_RoundTripsEveryKindOfPeer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input tg.InputPeerClass
		id    string
	}{
		{"user", &tg.InputPeerUser{UserID: 42, AccessHash: 99}, "user:42:99"},
		{"group", &tg.InputPeerChat{ChatID: 7}, "chat:7"},
		{"channel", &tg.InputPeerChannel{ChannelID: 5, AccessHash: -3}, "channel:5:-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := remoteID(tt.input); got != tt.id {
				t.Fatalf("remoteID = %q, want %q", got, tt.id)
			}

			back, err := inputPeer(tt.id)
			if err != nil {
				t.Fatal(err)
			}

			if remoteID(back) != tt.id {
				t.Errorf("inputPeer(%q) round-trips to %q", tt.id, remoteID(back))
			}
		})
	}
}

func TestInputPeer_RejectsMalformedIDs(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"", "user:42", "user:x:1", "chat:", "channel:5", "planet:1", "user:1:2:3", "user:00:0", "user:+5:1"} {
		if _, err := inputPeer(id); err == nil {
			t.Errorf("inputPeer(%q) succeeded", id)
		}
	}
}

// FuzzInputPeer checks that inputPeer never panics on a stored remote id,
// however it is malformed, and that every id remoteID can produce comes
// back unchanged.
func FuzzInputPeer(f *testing.F) {
	for _, id := range []string{"user:42:99", "chat:7", "channel:5:-3", "", "user:42", "user:x:1", "planet:1", "user:1:2:3"} {
		f.Add(id)
	}

	f.Fuzz(func(t *testing.T, id string) {
		peer, err := inputPeer(id)
		if err != nil {
			return
		}

		if got := remoteID(peer); got != id {
			t.Errorf("inputPeer(%q) round-trips to %q", id, got)
		}
	})
}
