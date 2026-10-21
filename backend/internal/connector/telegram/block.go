package telegram

// block.go implements connector.Blocker and connector.SpamReporter:
// blocking or unblocking a conversation's peer, and reporting it as
// spam, through Telegram's own contacts.block, contacts.unblock and
// messages.reportSpam calls.

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var (
	_ connector.Blocker      = (*Connector)(nil)
	_ connector.SpamReporter = (*Connector)(nil)
)

// SetBlocked blocks or unblocks conv's peer with Telegram.
func (c *Connector) SetBlocked(ctx context.Context, conv domain.Conversation, blocked bool) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	if blocked {
		_, err = api.ContactsBlock(ctx, &tg.ContactsBlockRequest{ID: peer})
	} else {
		_, err = api.ContactsUnblock(ctx, &tg.ContactsUnblockRequest{ID: peer})
	}
	if err != nil {
		return fmt.Errorf("telegram: set blocked: %w", err)
	}

	return nil
}

// ReportSpam reports conv's peer to Telegram as spam.
func (c *Connector) ReportSpam(ctx context.Context, conv domain.Conversation) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	if _, err := api.MessagesReportSpam(ctx, peer); err != nil {
		return fmt.Errorf("telegram: report spam: %w", err)
	}

	return nil
}
