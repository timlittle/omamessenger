package domain_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSanitizeIncoming_RecapsReplyQuoteText confirms a reply quote's
// own Text is cut to domain.ExcerptLength the same way store.resolveReply
// already shortens it when it is first written, so a row an earlier
// release stored before that cap existed - or one a connector wrote
// with every reply field already filled in, which resolveReply then
// never shortens at all - is repaired on every read, the same defence
// SanitizeIncoming already gives ReplyTo.SenderName.
func TestSanitizeIncoming_RecapsReplyQuoteText(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("q", domain.ExcerptLength+50)
	m := domain.Message{
		Text:    "hi",
		ReplyTo: &domain.Reply{RemoteID: "r1", SenderName: "Quoted", Text: long},
	}

	got := domain.SanitizeIncoming(m)
	want := domain.Excerpt(long)
	if got.ReplyTo.Text != want {
		t.Errorf("SanitizeIncoming left ReplyTo.Text as %q, want %q", got.ReplyTo.Text, want)
	}
	if n := utf8.RuneCountInString(got.ReplyTo.Text); n > domain.ExcerptLength+1 {
		t.Errorf("SanitizeIncoming kept ReplyTo.Text at %d runes, want at most %d", n, domain.ExcerptLength+1)
	}
}

// TestSanitizeIncoming_DropsAnOversizedReplyRemoteID confirms a reply
// quote's own RemoteID is dropped once it runs past
// domain.MaxRemoteIDLength, never truncated: WhatsApp reads it straight
// from a message's own encrypted ContextInfo.StanzaID, which carries
// whatever the sender's client put there with no length limit of its
// own, so a hostile sender could otherwise reference an implausibly
// long id. SenderName and Text stay: a quote preview is still worth
// showing even once it can no longer be resolved or linked back to the
// message it claims to answer (see store.resolveReply's own empty-id
// guard).
func TestSanitizeIncoming_DropsAnOversizedReplyRemoteID(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("r", domain.MaxRemoteIDLength+1)
	m := domain.Message{
		Text:    "hi",
		ReplyTo: &domain.Reply{RemoteID: huge, SenderName: "Quoted", Text: "quoted text"},
	}

	got := domain.SanitizeIncoming(m)
	if got.ReplyTo.RemoteID != "" {
		t.Errorf("SanitizeIncoming kept an oversized ReplyTo.RemoteID of %d bytes, want it dropped", len(huge))
	}
	if got.ReplyTo.SenderName != "Quoted" || got.ReplyTo.Text != "quoted text" {
		t.Errorf("SanitizeIncoming changed the reply quote's other fields: %+v", got.ReplyTo)
	}
}
