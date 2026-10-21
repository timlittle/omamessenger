package app

// block.go implements the UI's request to block or unblock a direct
// conversation's contact, and to report one as spam, matching what
// the official WhatsApp and Telegram apps do: reporting spam also
// blocks the contact and hides the conversation from the standard
// list, so it stops showing without a second step. Both call the
// account's Blocker first, through the connector that the helper
// wired up (see connector.Manager.SetBlocked); the local blocked flag
// is only persisted once the service has confirmed it, so a failed
// block never looks like a successful one. ReportSpam additionally
// asks the SpamReporter the account's connector implements, when it
// has one - WhatsApp's own API offers no equivalent report, so its
// connector implements only Blocker (see docs/decisions.md) - and
// hides the conversation with SetHidden rather than SetArchived,
// because archived is one of the two fields a service's own dialog
// sync can revert through Ingest.Organized, and nothing here reports
// this archive to the service for that sync to agree with.

import (
	"context"
	"errors"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// BlockContact blocks or unblocks the contact behind a direct
// conversation, through its service. Only a direct conversation has
// the one contact a block applies to; a group fails with
// ErrInvalidInput.
func (c *Commands) BlockContact(ctx context.Context, conversationID string, blocked bool) (domain.Conversation, error) {
	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conv.Kind != domain.KindDirect {
		return domain.Conversation{}, fmt.Errorf("%w: only a direct chat's contact can be blocked", ErrInvalidInput)
	}

	if err := blockError(c.blocker.SetBlocked(ctx, conv, blocked)); err != nil {
		return domain.Conversation{}, err
	}

	if err := c.store.SetBlocked(ctx, conversationID, blocked); err != nil {
		return domain.Conversation{}, err
	}

	return c.events.publishConversation(ctx, conversationID)
}

// ReportSpam reports a direct conversation's contact as spam, through
// the service when it supports reporting one, then blocks the contact
// and hides the conversation from the standard list. A group fails
// with ErrInvalidInput, the same as BlockContact.
func (c *Commands) ReportSpam(ctx context.Context, conversationID string) (domain.Conversation, error) {
	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conv.Kind != domain.KindDirect {
		return domain.Conversation{}, fmt.Errorf("%w: only a direct chat can be reported as spam", ErrInvalidInput)
	}

	if err := c.spamReporter.ReportSpam(ctx, conv); err != nil {
		return domain.Conversation{}, err
	}
	if err := blockError(c.blocker.SetBlocked(ctx, conv, true)); err != nil {
		return domain.Conversation{}, err
	}

	if err := c.store.SetBlocked(ctx, conversationID, true); err != nil {
		return domain.Conversation{}, err
	}
	if err := c.store.SetHidden(ctx, conversationID, true); err != nil {
		return domain.Conversation{}, err
	}

	return c.events.publishConversation(ctx, conversationID)
}

// blockError turns a connector's missing Blocker into ErrInvalidInput
// with a safe message the UI can show; anything else, such as a
// network failure, is returned unchanged so it becomes a fixed
// internal error.
func blockError(err error) error {
	if errors.Is(err, connector.ErrNoBlocker) {
		return fmt.Errorf("%w: this service does not support blocking contacts", ErrInvalidInput)
	}

	return err
}
