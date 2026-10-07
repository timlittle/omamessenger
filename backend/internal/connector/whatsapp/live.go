package whatsapp

// This file handles the events whatsmeow keeps delivering while
// connected: incoming and outgoing-from-elsewhere messages, reactions,
// edits, revokes, typing, and self-read receipts from the account's
// other devices. Outgoing delivery and read receipts for messages this
// connector itself sent are a different concern, left to lane B's own
// handler on the same events.Receipt case in events.go.

import (
	"context"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// handleMessage reports a live message: its content as an incoming or
// backfilled-from-another-device message, or, when it is actually one
// of WhatsApp's own protocol wrappers, the reaction, edit or revoke it
// carries instead.
func (c *Connector) handleMessage(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Message) {
	switch {
	case isReaction(e.Message):
		c.handleReaction(ctx, sink, e)
	case isRevoke(e.Message):
		c.handleRevoke(ctx, sink, e)
	case isEdit(e.Message):
		c.handleEdit(ctx, sink, e)
	default:
		c.handleContent(ctx, sink, dev, media, e)
	}
}

// handleContent reports a message's own content. An incoming message's
// conversation is reported first, so a brand-new chat is never dropped;
// an outgoing one WhatsApp reports from another of this account's
// devices skips that, since it carries no reliable name for an
// already-known chat and would otherwise overwrite a good title with a
// generic one. Such a message is history: it never notifies.
func (c *Connector) handleContent(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Message) {
	remote := remoteID(e.Info.Chat)
	if !e.Info.IsFromMe {
		c.ensureChat(ctx, sink, dev, e.Info)
	}

	m := message(e.Info, e.Message)
	saveMediaRef(ctx, media, remote, m.RemoteID, e.Message)

	if e.Info.IsFromMe {
		sink.History(ctx, c.account.ID, remote, m)
		return
	}

	sink.Incoming(ctx, c.account.ID, remote, m)
}

// ensureChat reports info's chat as a conversation, resolving a group's
// name when this connector has not seen it yet, so a message in a chat
// that history sync has not reached still gets somewhere to live.
func (c *Connector) ensureChat(ctx context.Context, sink connector.Sink, dev device, info types.MessageInfo) {
	if kindFor(info.Chat) == domain.KindGroup {
		name := c.resolveGroupName(ctx, dev, info.Chat)
		sink.Conversation(ctx, domain.Conversation{
			AccountID: c.account.ID, RemoteID: remoteID(info.Chat), Kind: domain.KindGroup, Title: name,
		})

		return
	}

	sink.Conversation(ctx, directConversation(c.account.ID, info))
}

// handleReaction folds a live reaction change into its message's full
// tally and reports the result.
func (c *Connector) handleReaction(ctx context.Context, sink connector.Sink, e *events.Message) {
	remote := remoteID(e.Info.Chat)
	messageRemoteID, emoji := reaction(e.Message)

	tally := c.reactTo(remote, messageRemoteID, reactorKey(e.Info), emoji)
	sink.Reacted(ctx, c.account.ID, remote, messageRemoteID, tally)
}

// reactorKey identifies who a reaction or receipt belongs to: "self"
// for this account, from whichever of its devices, or the sender's
// remote id otherwise.
func reactorKey(info types.MessageInfo) string {
	if info.IsFromMe {
		return "self"
	}

	return remoteID(info.Sender)
}

// handleRevoke reports a message deleted from the service.
func (c *Connector) handleRevoke(ctx context.Context, sink connector.Sink, e *events.Message) {
	remote := remoteID(e.Info.Chat)
	sink.Deleted(ctx, c.account.ID, []string{remote}, []string{revoke(e.Message)})
}

// handleEdit reports a message changed after it was sent.
func (c *Connector) handleEdit(ctx context.Context, sink connector.Sink, e *events.Message) {
	sink.Edited(ctx, c.account.ID, remoteID(e.Info.Chat), edit(e.Info, e.Message))
}

// handleChatPresence reports someone typing or stopping, naming them
// only in a group: a direct chat's single header has no room for a
// name, matching how this helper's other connector reports it.
func (c *Connector) handleChatPresence(ctx context.Context, sink connector.Sink, e *events.ChatPresence) {
	name := ""
	if e.IsGroup {
		name = c.nameFor(remoteID(e.Sender))
	}

	sink.Typing(ctx, c.account.ID, remoteID(e.Chat), name, typingActive(e.State))
}

// handleReceipt reports the one kind of receipt that is ours to handle
// here: this account reading a chat on one of its other devices, which
// WhatsApp never breaks down by message, so the whole conversation is
// marked read. A receipt about a message this account sent (IsFromMe
// false here, since then the chat partner is the one acknowledging it)
// is lane B's outgoing delivery and read progress instead.
func (c *Connector) handleReceipt(ctx context.Context, sink connector.Sink, e *events.Receipt) {
	if !e.IsFromMe {
		return
	}

	if e.Type != types.ReceiptTypeRead && e.Type != types.ReceiptTypeReadSelf {
		return
	}

	sink.Unread(ctx, c.account.ID, remoteID(e.Chat), 0)
}

// handlePin reports a chat pinned or unpinned from the phone, merging it
// with whichever archived state this connector last knew for it.
func (c *Connector) handlePin(ctx context.Context, sink connector.Sink, e *events.Pin) {
	remote := remoteID(e.JID)
	pinned := e.Action.GetPinned()

	state := c.setOrganized(remote, &pinned, nil)
	sink.Organized(ctx, c.account.ID, remote, state.pinned, state.archived)
}

// handleArchive reports a chat archived or unarchived from the phone,
// merging it with whichever pinned state this connector last knew for
// it.
func (c *Connector) handleArchive(ctx context.Context, sink connector.Sink, e *events.Archive) {
	remote := remoteID(e.JID)
	archived := e.Action.GetArchived()

	state := c.setOrganized(remote, nil, &archived)
	sink.Organized(ctx, c.account.ID, remote, state.pinned, state.archived)
}
