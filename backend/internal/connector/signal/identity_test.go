package signal

import (
	"testing"

	"github.com/google/uuid"
)

// TestMessageRemoteID checks the remote id a message's sender and
// sent timestamp give it, since reactions, edits, deletes and poll
// votes all have to reconstruct the same string to target it.
func TestMessageRemoteID(t *testing.T) {
	t.Parallel()

	aci := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	got := messageRemoteID(aci, 1700000000000)
	want := "11111111-1111-1111-1111-111111111111:1700000000000"
	if got != want {
		t.Errorf("messageRemoteID() = %q, want %q", got, want)
	}
}

// TestIsGroupChat checks the length heuristic that tells a group's
// chat id apart from a direct chat's, matching the real Signal
// bridge's own makePortalKey rule.
func TestIsGroupChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		chatID string
		want   bool
	}{
		{"aci", uuid.New().String(), false},
		{"pni", "PNI:" + uuid.New().String(), false},
		{"group", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isGroupChat(tt.chatID); got != tt.want {
				t.Errorf("isGroupChat(%q) = %t, want %t", tt.chatID, got, tt.want)
			}
		})
	}
}

// TestParseACI_RejectsGarbage checks that a malformed id is reported
// as an error rather than silently producing the zero value, since a
// live reaction or vote's target author comes straight off the wire.
func TestParseACI_RejectsGarbage(t *testing.T) {
	t.Parallel()

	if _, err := parseACI("not-a-uuid"); err == nil {
		t.Error("parseACI(garbage) succeeded, want an error")
	}
}

// FuzzParseACI checks that no input, however malformed, makes parseACI
// panic: it only ever sees bytes signalmeow decoded off the wire from
// another Signal client, which this connector does not otherwise
// trust.
func FuzzParseACI(f *testing.F) {
	f.Add("")
	f.Add(uuid.New().String())
	f.Add("PNI:" + uuid.New().String())

	f.Fuzz(func(t *testing.T, s string) {
		_, _ = parseACI(s)
	})
}

// FuzzAnchorFor checks that no message remote id, however malformed,
// makes anchorFor panic: LoadOlder is given whatever the UI already
// stored as beforeRemoteID, which this connector does not re-verify.
func FuzzAnchorFor(f *testing.F) {
	f.Add("")
	f.Add(messageRemoteID(uuid.New(), 1700000000000))
	f.Add("garbage")
	f.Add(":")

	f.Fuzz(func(t *testing.T, s string) {
		_ = anchorFor(s)
	})
}
