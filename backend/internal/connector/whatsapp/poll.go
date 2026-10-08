package whatsapp

// poll.go normalizes WhatsApp's poll creation messages into ours. A
// poll's vote tally works differently: WhatsApp reports it through a
// separate, encrypted PollUpdateMessage with no option text of its own
// (see vote.go and pollstore.go), rather than alongside the poll's own
// creation the way Telegram's does.

import (
	"context"
	"encoding/hex"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// pollOptionID is the stable id this connector gives a poll option: the
// hex of the SHA-256 hash WhatsApp's own vote protocol identifies it by
// (see whatsmeow.HashPollOptions), so a vote's selected option ids
// never need a lookup table of their own to turn back into the hash a
// vote message carries.
func pollOptionID(name string) string {
	return hex.EncodeToString(whatsmeow.HashPollOptions([]string{name})[0])
}

// pollCreationMedia normalizes a poll creation message into ours, with
// no votes yet: they arrive later, through a separate PollUpdateMessage
// (see vote.go).
func pollCreationMedia(p *waE2E.PollCreationMessage) *domain.Media {
	names := p.GetOptions()
	options := make([]domain.PollOption, len(names))
	for i, o := range names {
		options[i] = domain.PollOption{ID: pollOptionID(o.GetOptionName()), Text: o.GetOptionName()}
	}

	return &domain.Media{Kind: domain.MediaPoll, Poll: &domain.Poll{
		Question: p.GetName(), Options: options, MultipleChoice: p.GetSelectableOptionsCount() != 1,
	}}
}

// savePoll remembers a poll creation message's question and options
// for later, if msg carries one, so a vote can be shown against the
// right option and tallied even after a restart (see pollstore.go).
// Saving is best effort, the same as saveMediaRef: a failure only means
// this one poll's later votes cannot be resolved, not that the message
// itself fails to report.
func savePoll(ctx context.Context, media *mediaStore, conversationRemoteID, messageRemoteID string, msg *waE2E.Message) {
	if media == nil {
		return
	}

	p := pollCreation(msg)
	if p == nil {
		return
	}

	_ = media.putPoll(ctx, conversationRemoteID, messageRemoteID, *pollCreationMedia(p).Poll)
}
