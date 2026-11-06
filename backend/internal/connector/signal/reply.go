package signal

// reply.go builds the Quote an outgoing DataMessage carries when m
// replies to another message, so Signal's own clients render it above
// the new message.

import (
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// outgoingQuote builds the Quote for reply: the quoted message's own
// sent timestamp (reply.RemoteID is "aci:timestamp", see
// parseMessageRemoteID) and author, plus its text when known. A reply
// whose own remote id does not parse - one quoting a message from
// before this connector tracked ids this way - is quoted by author and
// text alone, with no id, since Signal's clients still thread it by
// the ContextInfo text.
func outgoingQuote(reply *domain.Reply) *signalpb.DataMessage_Quote {
	quote := &signalpb.DataMessage_Quote{}
	if reply.Text != "" {
		quote.Text = strp(reply.Text)
	}

	if _, timestamp, err := parseMessageRemoteID(reply.RemoteID); err == nil {
		quote.Id = u64p(timestamp)
	}
	if aci, err := parseACI(reply.SenderID); err == nil {
		quote.AuthorAci = strp(reply.SenderID)
		quote.AuthorAciBinary = aci[:]
	}

	return quote
}
