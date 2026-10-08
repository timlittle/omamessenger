package app

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// retireOutgoingAttachment drops m's outgoing attachment once its
// delivery is confirmed and it will never be retried. The timeline may
// still be showing the attachment straight from that outgoing copy (see
// Commands.localAttachment), so it is moved into the downloaded-media
// cache first, under the same name a download would use, so it keeps
// displaying from there instead of being fetched back from the service.
// Either step failing is logged, not fatal: at worst the attachment is
// downloaded again next time it is wanted, or its copy outlives this
// message a little longer than it needed to.
func (in *Ingest) retireOutgoingAttachment(ctx context.Context, m domain.Message) {
	if !m.Outgoing || m.Media == nil || in.outgoing == nil || !confirmedDelivery(m.Status) {
		return
	}

	if in.cache != nil {
		path := in.outgoing.Path(m.ID, m.Media.FileName)
		if err := in.cache.Adopt(ctx, mediaFileName(m), path); err != nil && in.logger != nil {
			in.logger.Printf("media: keep sent attachment failed (reason=adopt)")
		}
	}

	if err := in.outgoing.Remove(ctx, m.ID, m.Media.FileName); err != nil && in.logger != nil {
		in.logger.Printf("media: remove sent attachment failed")
	}
}

// confirmedDelivery reports whether status is a delivery state that will
// never be retried, so any outgoing attachment for it is only needed
// until now.
func confirmedDelivery(status string) bool {
	return status == domain.StatusSent || status == domain.StatusDelivered || status == domain.StatusRead
}

// removeOutgoingAttachment drops m's outgoing attachment copy once its
// message is deleted, the same reasoning as retireOutgoingAttachment:
// a deleted message will never be retried, so its copy, if it still has
// one, is only needed until now. Only this account's own sends ever have
// a copy there, so an incoming message, whose file name its sender
// chose, never reaches the outgoing area at all.
func (c *Commands) removeOutgoingAttachment(ctx context.Context, m domain.Message) {
	if !m.Outgoing || m.Media == nil || c.outgoing == nil {
		return
	}

	if err := c.outgoing.Remove(ctx, m.ID, m.Media.FileName); err != nil && c.logger != nil {
		c.logger.Printf("media: remove deleted attachment failed")
	}
}
