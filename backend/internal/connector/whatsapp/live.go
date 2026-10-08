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
		c.handleReaction(ctx, sink, dev, e)
	case isRevoke(e.Message):
		c.handleRevoke(ctx, sink, dev, e)
	case isEdit(e.Message):
		c.handleEdit(ctx, sink, dev, e)
	case isContentless(e.Message):
		// WhatsApp's own protocol and system notices carry nothing a
		// person sent; see isContentless. Nothing is reported for one.
		logDropped(reasonContentless)
	default:
		c.handleContent(ctx, sink, dev, media, e)
	}
}

// handleContent reports a message's own content. An incoming message's
// conversation is reported first, so a brand-new chat is never dropped,
// and it is noted as unread so a later MarkRead for this conversation
// tells WhatsApp about it; an outgoing one WhatsApp reports from another
// of this account's devices skips both, since it carries no reliable
// name for an already-known chat and would otherwise overwrite a good
// title with a generic one, and it was never unread to begin with. The
// account's own self-chat is the one exception: every message in it is
// "from me", since there is no one else to send it, so it is the only
// outgoing chat this still ensures exists. System JIDs history sync or
// a live event can still deliver, such as the status broadcast, carry
// nothing worth showing and are dropped outright (see isSystemJID).
// Such a message is history: it never notifies.
func (c *Connector) handleContent(ctx context.Context, sink connector.Sink, dev device, media *mediaStore, e *events.Message) {
	if isSystemJID(e.Info.Chat) {
		return
	}

	if field, ok := unknownContentKind(unwrap(e.Message)); ok {
		logUnknownKind(field)
	}

	remote := chatID(ctx, dev, e.Info.Chat)
	if !e.Info.IsFromMe || dev.isSelfChat(ctx, e.Info.Chat) {
		c.ensureChat(ctx, sink, dev, e.Info)
	}

	m := c.improvedSenderName(message(ctx, dev, e.Info, e.Message))
	saveMediaRef(ctx, media, remote, m.RemoteID, e.Message)
	saveMessageKey(ctx, media, remote, m.RemoteID, messageKey{senderID: senderKeyID(e.Info), fromMe: e.Info.IsFromMe})

	// A redelivery of a message first seen as an UndecryptableMessage
	// (see handleUndecryptable) carries the same id: replace its
	// placeholder rather than report it a second time.
	if c.resolveUndecryptable(remote, m.RemoteID) {
		if e.UnavailableRequestID != "" {
			// whatsmeow sets this when the redelivery came from the
			// primary phone answering AutomaticMessageRerequestFromPhone's
			// request, rather than the original sender's own retry.
			logPhoneResend()
		}
		sink.Edited(ctx, c.account.ID, remote, m)
		return
	}

	if e.Info.IsFromMe {
		sink.History(ctx, c.account.ID, remote, m)
		return
	}

	c.notePendingRead(remote, m.SenderID, m.RemoteID)
	sink.Incoming(ctx, c.account.ID, remote, m)
}

// ensureChat reports info's chat as a conversation: the account's own
// self-chat, a group, resolving its name when this connector has not
// seen it yet, or a direct chat, so a message in a chat that history
// sync has not reached still gets somewhere to live.
func (c *Connector) ensureChat(ctx context.Context, sink connector.Sink, dev device, info types.MessageInfo) {
	remote := chatID(ctx, dev, info.Chat)

	if dev.isSelfChat(ctx, info.Chat) {
		c.reportConversation(ctx, sink, domain.Conversation{
			AccountID: c.account.ID, RemoteID: remote, Kind: domain.KindDirect, Title: selfChatTitle,
		})

		return
	}

	if kindFor(info.Chat) == domain.KindGroup {
		name, members := c.resolveGroupName(ctx, dev, info.Chat, "", 0)
		c.reportConversation(ctx, sink, domain.Conversation{
			AccountID: c.account.ID, RemoteID: remote, Kind: domain.KindGroup, Title: name, Members: members,
		})

		return
	}

	title := c.resolveDirectTitle(ctx, dev, info.Chat, info.PushName)
	c.reportConversation(ctx, sink, domain.Conversation{
		AccountID: c.account.ID, RemoteID: remote, Kind: domain.KindDirect, Title: title,
	})
}

// handleReaction folds a live reaction change into its message's full
// tally and reports the result.
func (c *Connector) handleReaction(ctx context.Context, sink connector.Sink, dev device, e *events.Message) {
	remote := chatID(ctx, dev, e.Info.Chat)
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
func (c *Connector) handleRevoke(ctx context.Context, sink connector.Sink, dev device, e *events.Message) {
	remote := chatID(ctx, dev, e.Info.Chat)
	sink.Deleted(ctx, c.account.ID, []string{remote}, []string{revoke(e.Message)})
}

// handleEdit reports a message changed after it was sent.
func (c *Connector) handleEdit(ctx context.Context, sink connector.Sink, dev device, e *events.Message) {
	m := c.improvedSenderName(edit(ctx, dev, e.Info, e.Message))
	sink.Edited(ctx, c.account.ID, chatID(ctx, dev, e.Info.Chat), m)
}

// handleChatPresence reports someone typing or stopping, naming them
// only in a group: a direct chat's single header has no room for a
// name, matching how this helper's other connector reports it.
func (c *Connector) handleChatPresence(ctx context.Context, sink connector.Sink, dev device, e *events.ChatPresence) {
	name := ""
	if e.IsGroup {
		name = c.nameFor(remoteID(e.Sender))
	}

	sink.Typing(ctx, c.account.ID, chatID(ctx, dev, e.Chat), name, typingActive(e.State))
}

// handleReceipt reports the one kind of receipt that is ours to handle
// here: this account reading a chat on one of its other devices, which
// WhatsApp never breaks down by message, so the whole conversation is
// marked read. A receipt about a message this account sent (IsFromMe
// false here, since then the chat partner is the one acknowledging it)
// is lane B's outgoing delivery and read progress instead.
func (c *Connector) handleReceipt(ctx context.Context, sink connector.Sink, dev device, e *events.Receipt) {
	if !e.IsFromMe {
		return
	}

	if e.Type != types.ReceiptTypeRead && e.Type != types.ReceiptTypeReadSelf {
		return
	}

	sink.Unread(ctx, c.account.ID, chatID(ctx, dev, e.Chat), 0)
}

// handlePin reports a chat pinned or unpinned from the phone, merging it
// with whichever archived state this connector last knew for it, and
// marks pinned as confirmed by app state so a later history sync's own
// snapshot can never revert it (see setOrganizedFromAppState).
func (c *Connector) handlePin(ctx context.Context, sink connector.Sink, dev device, e *events.Pin) {
	remote := chatID(ctx, dev, e.JID)
	pinned := e.Action.GetPinned()

	c.clearLocalOrganize(remote) // a live echo is WhatsApp's own current state, always trusted over a pending local change
	state := c.setOrganizedFromAppState(remote, &pinned, nil)
	sink.Organized(ctx, c.account.ID, remote, state.pinned, state.archived)
}

// handleArchive reports a chat archived or unarchived from the phone,
// merging it with whichever pinned state this connector last knew for
// it, and marks archived as confirmed by app state so a later history
// sync's own snapshot can never revert it (see setOrganizedFromAppState).
func (c *Connector) handleArchive(ctx context.Context, sink connector.Sink, dev device, e *events.Archive) {
	remote := chatID(ctx, dev, e.JID)
	archived := e.Action.GetArchived()

	c.clearLocalOrganize(remote) // a live echo is WhatsApp's own current state, always trusted over a pending local change
	state := c.setOrganizedFromAppState(remote, nil, &archived)
	sink.Organized(ctx, c.account.ID, remote, state.pinned, state.archived)
}

// undecryptablePlaceholder stands in for a message whatsmeow could not
// decrypt, in WhatsApp's own wording style, until either the real
// content replaces it (see resolveUndecryptable) or it is accepted as
// permanently lost.
const undecryptablePlaceholder = "Waiting for this message"

// handleUndecryptable reports a placeholder for a message whatsmeow
// received but could not decrypt. This is most often one of this
// account's own other linked devices (such as a bot replying in the
// self-chat) sending before a session with that device exists yet.
// whatsmeow first asks the sender to retry on its own; if the sender
// never answers, AutomaticMessageRerequestFromPhone (set in device.go)
// has it ask the primary phone directly instead. Either way, a
// successful redelivery arrives as an ordinary events.Message with the
// same id, which handleContent then uses to replace this placeholder
// (see markUndecryptable) instead of reporting it twice. Nothing is
// reported for a system JID, the same as a real message.
func (c *Connector) handleUndecryptable(ctx context.Context, sink connector.Sink, dev device, e *events.UndecryptableMessage) {
	logUndecryptable(e.IsUnavailable, e.DecryptFailMode)

	if isSystemJID(e.Info.Chat) {
		return
	}

	remote := chatID(ctx, dev, e.Info.Chat)
	if !e.Info.IsFromMe || dev.isSelfChat(ctx, e.Info.Chat) {
		c.ensureChat(ctx, sink, dev, e.Info)
	}
	c.markUndecryptable(remote, e.Info.ID)

	m := domain.Message{
		RemoteID: e.Info.ID,
		Text:     undecryptablePlaceholder,
		Outgoing: e.Info.IsFromMe,
		Status:   domain.StatusReceived,
		Created:  e.Info.Timestamp.UnixMilli(),
	}

	if e.Info.IsFromMe {
		m.SenderID, m.SenderName, m.Status = "self", "You", domain.StatusSent
		sink.History(ctx, c.account.ID, remote, m)
		return
	}

	m.SenderID, m.SenderName = remoteID(e.Info.Sender), senderName(e.Info)
	m = c.improvedSenderName(m)
	c.notePendingRead(remote, m.SenderID, m.RemoteID)
	sink.Incoming(ctx, c.account.ID, remote, m)
}
