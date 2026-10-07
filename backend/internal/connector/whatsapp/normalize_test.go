package whatsapp

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestRemoteID_IsTheBareJIDWithoutADevice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		jid  types.JID
		want string
	}{
		{"phone number", types.NewJID("15551234567", types.DefaultUserServer), "15551234567@s.whatsapp.net"},
		{"group", types.NewJID("12345-1600000000", types.GroupServer), "12345-1600000000@g.us"},
		{"linked id", types.NewJID("987654", types.HiddenUserServer), "987654@lid"},
		{"device dropped", types.NewADJID("15551234567", 0, 5), "15551234567@s.whatsapp.net"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := remoteID(tt.jid); got != tt.want {
				t.Errorf("remoteID(%+v) = %q, want %q", tt.jid, got, tt.want)
			}
		})
	}
}

func TestJIDFromRemoteID_RoundTripsACanonicalID(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"15551234567@s.whatsapp.net", "12345-1600000000@g.us", "987654@lid"} {
		jid, err := jidFromRemoteID(id)
		if err != nil {
			t.Fatalf("jidFromRemoteID(%q) error: %v", id, err)
		}

		if got := remoteID(jid); got != id {
			t.Errorf("jidFromRemoteID(%q) round-tripped to %q", id, got)
		}
	}
}

func TestJIDFromRemoteID_RejectsNonCanonicalForms(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"15551234567:5@s.whatsapp.net", // a device suffix, which remoteID strips
		"15551234567.1@s.whatsapp.net", // an agent suffix
		"not a jid at all",
	}

	for _, id := range tests {
		if _, err := jidFromRemoteID(id); err == nil {
			t.Errorf("jidFromRemoteID(%q) accepted a non-canonical id", id)
		}
	}
}

func TestJIDFromRemoteID_ErrorNeverIncludesTheUserSegment(t *testing.T) {
	t.Parallel()

	// A rejected id on the default server has a phone number as its
	// user segment, which must never reach an error message that might
	// be logged.
	_, err := jidFromRemoteID("15551234567:5@s.whatsapp.net")
	if err == nil {
		t.Fatal("want an error for a non-canonical id")
	}

	if got := err.Error(); contains(got, "15551234567") {
		t.Errorf("jidFromRemoteID error = %q, leaked the user segment", got)
	}
}

// FuzzJIDFromRemoteID checks that jidFromRemoteID never panics on an
// arbitrary string, and that whatever it accepts round-trips back to
// the exact string given, per its canonical-only contract.
func FuzzJIDFromRemoteID(f *testing.F) {
	for _, seed := range []string{
		"", "15551234567@s.whatsapp.net", "15551234567:5@s.whatsapp.net",
		"12345-1600000000@g.us", "987654@lid", "@g.us", "no-at-sign", "a.b:c@d",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, id string) {
		jid, err := jidFromRemoteID(id)
		if err == nil && remoteID(jid) != id {
			t.Errorf("jidFromRemoteID(%q) accepted a non-canonical id", id)
		}
	})
}

func TestChatID_CollapsesTheSelfChatRegardlessOfForm(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("111222", types.HiddenUserServer)
	dev := &fakeDevice{selfJID: phone, selfLID: lid}

	for _, jid := range []types.JID{phone, lid} {
		if got, want := chatID(t.Context(), dev, jid), remoteID(phone); got != want {
			t.Errorf("chatID(%v) = %q, want %q", jid, got, want)
		}
	}
}

func TestChatID_ResolvesAMappedLIDChatToItsPhoneJID(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15557654321", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev := &fakeDevice{lidPhones: map[string]types.JID{lid.String(): phone}}

	if got, want := chatID(t.Context(), dev, lid), remoteID(phone); got != want {
		t.Errorf("chatID(mapped lid) = %q, want %q", got, want)
	}
}

func TestChatID_FallsBackToTheBareLIDWhenUnmapped(t *testing.T) {
	t.Parallel()

	lid := types.NewJID("987654", types.HiddenUserServer)
	dev := &fakeDevice{}

	if got, want := chatID(t.Context(), dev, lid), remoteID(lid); got != want {
		t.Errorf("chatID(unmapped lid) = %q, want %q", got, want)
	}
}

func TestKindFor_GroupServerIsGroupEverythingElseIsDirect(t *testing.T) {
	t.Parallel()

	if got := kindFor(types.NewJID("1", types.GroupServer)); got != domain.KindGroup {
		t.Errorf("kindFor(group) = %q, want %q", got, domain.KindGroup)
	}

	if got := kindFor(types.NewJID("1", types.DefaultUserServer)); got != domain.KindDirect {
		t.Errorf("kindFor(user) = %q, want %q", got, domain.KindDirect)
	}
}

func TestMuted_ComparesTheEndTimeToNow(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_800_000_000, 0)

	tests := []struct {
		name    string
		endTime uint64
		want    bool
	}{
		{"never muted", 0, false},
		{"muted until later", uint64(now.Unix()) + 3600, true},
		{"mute already expired", uint64(now.Unix()) - 3600, false},
	}

	for _, tt := range tests {
		if got := muted(tt.endTime, now); got != tt.want {
			t.Errorf("muted(%s) = %t, want %t", tt.name, got, tt.want)
		}
	}
}

func TestConversationFromSync_DirectAndGroup(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_800_000_000, 0)

	tests := []struct {
		name string
		conv *waHistorySync.Conversation
		want domain.Conversation
	}{
		{
			"direct chat",
			&waHistorySync.Conversation{
				ID: strPtr("15551234567@s.whatsapp.net"), Name: strPtr("Nadia"),
				UnreadCount: u32(3), Pinned: u32(1), Archived: boolPtr(false),
			},
			domain.Conversation{
				AccountID: "wa", RemoteID: "15551234567@s.whatsapp.net", Kind: domain.KindDirect,
				Title: "Nadia", Unread: 3, Pinned: true,
			},
		},
		{
			"group with a display name",
			&waHistorySync.Conversation{
				ID: strPtr("12345-1600000000@g.us"), DisplayName: strPtr("Climbing Crew"),
				Participant: []*waHistorySync.GroupParticipant{
					{UserJID: strPtr("1@s.whatsapp.net")}, {UserJID: strPtr("2@s.whatsapp.net")},
				},
				Archived: boolPtr(true),
			},
			domain.Conversation{
				AccountID: "wa", RemoteID: "12345-1600000000@g.us", Kind: domain.KindGroup,
				Title: "Climbing Crew", Members: 2, Archived: true,
			},
		},
		{
			"muted conversation",
			&waHistorySync.Conversation{
				ID: strPtr("1@s.whatsapp.net"), MuteEndTime: u64(uint64(now.Unix()) + 3600),
			},
			domain.Conversation{AccountID: "wa", RemoteID: "1@s.whatsapp.net", Kind: domain.KindDirect, Muted: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := conversationFromSync("wa", tt.conv, now)
			if !ok || got != tt.want {
				t.Errorf("conversationFromSync = %+v, %t; want %+v", got, ok, tt.want)
			}
		})
	}
}

func TestConversationFromSync_RejectsAnUnparseableID(t *testing.T) {
	t.Parallel()

	if _, ok := conversationFromSync("wa", &waHistorySync.Conversation{}, time.Now()); ok {
		t.Error("a conversation with no id became a conversation")
	}
}

// FuzzConversationFromSync checks that conversationFromSync never
// panics on a history sync conversation decoded from arbitrary bytes,
// which is what a compromised or buggy sync blob would deliver.
func FuzzConversationFromSync(f *testing.F) {
	seed, _ := proto.Marshal(&waHistorySync.Conversation{ID: strPtr("1@s.whatsapp.net"), Name: strPtr("Nadia")})
	f.Add(seed)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		var c waHistorySync.Conversation
		if err := proto.Unmarshal(data, &c); err != nil {
			return
		}

		conversationFromSync("wa", &c, time.Now())
	})
}

func TestTitleFallback_PhoneNumberOrNeutralLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		jid  types.JID
		want string
	}{
		{"phone JID", types.NewJID("15551234567", types.DefaultUserServer), "+15551234567"},
		{"LID", types.NewJID("987654", types.HiddenUserServer), "Unknown contact"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := titleFallback(tt.jid); got != tt.want {
				t.Errorf("titleFallback(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsSystemJID_RecognisesNonConversationJIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		jid  types.JID
		want bool
	}{
		{"status broadcast", types.StatusBroadcastJID, true},
		{"the 0 system account", types.PSAJID, true},
		{"a broadcast list", types.NewJID("123456", types.BroadcastServer), true},
		{"a newsletter", types.NewJID("1", types.NewsletterServer), true},
		{"a direct chat", types.NewJID("15551234567", types.DefaultUserServer), false},
		{"a group", types.NewJID("1-2", types.GroupServer), false},
		{"a LID", types.NewJID("987654", types.HiddenUserServer), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isSystemJID(tt.jid); got != tt.want {
				t.Errorf("isSystemJID(%s) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}

func TestContactDisplayName_PrefersSavedNamesOverAPushName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info types.ContactInfo
		want string
	}{
		{"full name wins", types.ContactInfo{Found: true, FullName: "Nadia Rahman", PushName: "nads"}, "Nadia Rahman"},
		{"first name beats business and push", types.ContactInfo{Found: true, FirstName: "Nadia", BusinessName: "Acme", PushName: "nads"}, "Nadia"},
		{"business name beats push", types.ContactInfo{Found: true, BusinessName: "Acme Support", PushName: "nads"}, "Acme Support"},
		{"push name is the last resort", types.ContactInfo{Found: true, PushName: "nads"}, "nads"},
		{"nothing known", types.ContactInfo{Found: true}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := contactDisplayName(tt.info); got != tt.want {
				t.Errorf("contactDisplayName(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestContactFromPushName_NamesAContactOrReportsNone(t *testing.T) {
	t.Parallel()

	got, ok := contactFromPushName("wa", &waHistorySync.Pushname{ID: strPtr("1@s.whatsapp.net"), Pushname: strPtr("Nadia")})
	want := domain.Contact{AccountID: "wa", RemoteID: "1@s.whatsapp.net", Name: "Nadia"}
	if !ok || got != want {
		t.Errorf("contactFromPushName = %+v, %t; want %+v, true", got, ok, want)
	}

	if _, ok := contactFromPushName("wa", &waHistorySync.Pushname{ID: strPtr("1@s.whatsapp.net")}); ok {
		t.Error("a push name with no name became a contact")
	}

	if _, ok := contactFromPushName("wa", &waHistorySync.Pushname{Pushname: strPtr("Nadia")}); ok {
		t.Error("a push name with no id became a contact")
	}
}
