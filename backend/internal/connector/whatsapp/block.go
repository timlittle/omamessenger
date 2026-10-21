package whatsapp

// block.go implements connector.Blocker: blocking or unblocking a
// contact through WhatsApp's own account-wide block list
// (whatsmeow.Client.UpdateBlocklist, wrapped as device.updateBlocklist
// for testability the same way every other per-contact call here is).
// WhatsApp's API has no equivalent "report as spam" call of its own,
// so this connector does not implement connector.SpamReporter;
// app.Commands.ReportSpam still blocks the contact through this, and
// hides the conversation locally instead (see docs/decisions.md).

import (
	"context"
	"fmt"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Blocker = (*Connector)(nil)

// blockTimeout bounds how long SetBlocked waits for WhatsApp to accept
// a block or unblock, matching this connector's other per-contact
// calls such as SetPinned and SetArchived.
const blockTimeout = 30 * time.Second

// SetBlocked blocks or unblocks conv's contact with WhatsApp.
func (c *Connector) SetBlocked(ctx context.Context, conv domain.Conversation, blocked bool) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	jid, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: set blocked: %w", err)
	}

	blockCtx, cancel := context.WithTimeout(ctx, blockTimeout)
	defer cancel()
	if err := dev.updateBlocklist(blockCtx, jid, blocked); err != nil {
		return fmt.Errorf("whatsapp: set blocked: %w", err)
	}

	return nil
}
