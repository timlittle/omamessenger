package signal

import (
	"testing"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestOutgoingQuote_FillsTimestampAuthorAndText checks that a reply
// to a known message quotes its parsed timestamp, author and text.
func TestOutgoingQuote_FillsTimestampAuthorAndText(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	reply := &domain.Reply{
		RemoteID: messageRemoteID(author, 1700000000123),
		SenderID: author.String(),
		Text:     "quoted text",
	}

	quote := outgoingQuote(reply)
	if quote.GetId() != 1700000000123 {
		t.Errorf("quote.Id = %d, want 1700000000123", quote.GetId())
	}
	if quote.GetAuthorAci() != author.String() {
		t.Errorf("quote.AuthorAci = %q, want %q", quote.GetAuthorAci(), author.String())
	}
	if quote.GetText() != "quoted text" {
		t.Errorf("quote.Text = %q, want %q", quote.GetText(), "quoted text")
	}
}

// TestOutgoingQuote_UnparseableIDQuotesTextOnly checks that a reply
// whose remote id does not parse - one from before this connector
// tracked ids this way - still quotes by text alone, with no id or
// author, rather than failing.
func TestOutgoingQuote_UnparseableIDQuotesTextOnly(t *testing.T) {
	t.Parallel()

	reply := &domain.Reply{RemoteID: "not-a-remote-id", Text: "quoted text"}

	quote := outgoingQuote(reply)
	if quote.GetId() != 0 || quote.GetAuthorAci() != "" {
		t.Errorf("quote = %+v, want no id or author", quote)
	}
	if quote.GetText() != "quoted text" {
		t.Errorf("quote.Text = %q, want %q", quote.GetText(), "quoted text")
	}
}
