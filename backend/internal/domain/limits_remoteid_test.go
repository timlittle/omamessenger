package domain_test

// limits_remoteid_test.go covers domain.CapRemoteID and the two places
// it is applied: a mention's own UserID (CapMentions) and a reply
// quote's own RemoteID (SanitizeIncoming). Kept apart from
// limits_test.go, the same way limits_replyquote_test.go already is,
// so limits_test.go stays under this project's file-length gate.

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestCapRemoteID_DropsAnOversizedID confirms an id past
// MaxRemoteIDLength is dropped outright, never truncated: a cut id
// could read as a different, shorter real id it was never actually
// sent as, while a cleared one can never be mistaken for one.
func TestCapRemoteID_DropsAnOversizedID(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("1", domain.MaxRemoteIDLength+1)
	if got := domain.CapRemoteID(huge); got != "" {
		t.Errorf("CapRemoteID kept an id of %d bytes, want it dropped", len(huge))
	}
}

// TestCapRemoteID_LeavesAShortIDAlone confirms an id already within
// the cap passes through unchanged.
func TestCapRemoteID_LeavesAShortIDAlone(t *testing.T) {
	t.Parallel()

	if got := domain.CapRemoteID("15551234567@s.whatsapp.net"); got != "15551234567@s.whatsapp.net" {
		t.Errorf("CapRemoteID(%q) = %q, want it unchanged", "15551234567@s.whatsapp.net", got)
	}
}

// TestCapMentions_ClearsAnOversizedUserID covers a mention whose own
// UserID has no length limit of its own: WhatsApp reads a mention's
// JID straight from a message's own encrypted ContextInfo.MentionedJID,
// which whatsmeow's types.ParseJID parses with no length check at all,
// so a hostile sender can make it arbitrarily long, up to MaxMentions
// times in a single message. CapMentions must clear it the same
// drop-rather-than-truncate way CapURL already treats an oversized URL:
// a cut id could read as a different, shorter real one it was never
// actually sent as.
func TestCapMentions_ClearsAnOversizedUserID(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("9", domain.MaxRemoteIDLength+1) + "@s.whatsapp.net"
	mentions := []domain.Mention{
		{UserID: huge, Name: "Alex", Offset: 0, Length: 1},
	}

	got := domain.CapMentions(mentions, 1)
	if len(got) != 1 {
		t.Fatalf("CapMentions kept %d mentions, want 1", len(got))
	}
	if got[0].UserID != "" {
		t.Errorf("CapMentions kept an oversized UserID of %d bytes, want it dropped", len(huge))
	}
	if got[0].Name != "Alex" {
		t.Errorf("CapMentions changed Name to %q, want it untouched", got[0].Name)
	}
}

// TestSanitizeIncoming_ClearsAnOversizedMentionUserID confirms a
// mention's own UserID is cleared the same way CapMentions already
// clears one directly: this is the one path both a newly arrived
// message and a row read back out of the store both go through, so
// neither can carry a mention naming an implausibly long id, up to
// MaxMentions times over, the way CapMentions already bounded the
// mention's own Name but never its UserID before this change.
func TestSanitizeIncoming_ClearsAnOversizedMentionUserID(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("9", domain.MaxRemoteIDLength+1) + "@s.whatsapp.net"
	m := domain.Message{
		Text:     "@x",
		Mentions: []domain.Mention{{UserID: huge, Name: "x", Offset: 0, Length: 2}},
	}

	got := domain.SanitizeIncoming(m)
	if len(got.Mentions) != 1 {
		t.Fatalf("SanitizeIncoming kept %d mentions, want 1", len(got.Mentions))
	}
	if got.Mentions[0].UserID != "" {
		t.Errorf("SanitizeIncoming kept an oversized mention UserID of %d bytes, want it dropped", len(huge))
	}
}
