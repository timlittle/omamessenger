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
