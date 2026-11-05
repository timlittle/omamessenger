package signal

import (
	"context"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// handleEvent dispatches one signalmeow event to the sink, returning
// true once it has been dealt with, as signalmeow's own retry
// machinery expects.
func (c *Connector) handleEvent(ctx context.Context, sink connector.Sink, evt events.SignalEvent) bool {
	switch e := evt.(type) {
	case *events.ChatEvent:
		c.handleChatEvent(ctx, sink, e)
	case *events.ContactList:
		c.handleContactList(e)
	case *events.ACIFound:
		c.handleACIFound(ctx, sink, e)
	}

	return true
}

// handleChatEvent dispatches a chat event's own content: a text
// message, a typing indicator or an edit.
func (c *Connector) handleChatEvent(ctx context.Context, sink connector.Sink, e *events.ChatEvent) {
	switch content := e.Event.(type) {
	case *signalpb.DataMessage:
		c.handleDataMessage(ctx, sink, e.Info, content, false)
	case *signalpb.TypingMessage:
		c.handleTyping(ctx, sink, e.Info, content)
	case *signalpb.EditMessage:
		c.handleEdit(ctx, sink, e.Info, content)
	}
}

// handleDataMessage reports msg as a reaction, delete, poll action or
// ordinary message, whichever its own fields say it is.
func (c *Connector) handleDataMessage(ctx context.Context, sink connector.Sink, info events.MessageInfo, msg *signalpb.DataMessage, outgoing bool) {
	c.ensureConversation(ctx, sink, info)

	switch {
	case msg.GetDelete() != nil:
		c.handleDelete(ctx, sink, info, msg.GetDelete())
	case msg.GetReaction() != nil:
		c.handleReaction(ctx, sink, info, msg.GetReaction())
	case msg.GetPollVote() != nil:
		c.handlePollVote(ctx, sink, info, msg.GetPollVote())
	case msg.GetPollTerminate() != nil:
		c.handlePollTerminate(ctx, sink, info, msg.GetPollTerminate())
	default:
		c.handleMessage(ctx, sink, info, msg, outgoing)
	}
}

// handleMessage reports an ordinary incoming or synced-outgoing
// message, and the chat's disappearing timer when msg carries one.
func (c *Connector) handleMessage(ctx context.Context, sink connector.Sink, info events.MessageInfo, msg *signalpb.DataMessage, outgoing bool) {
	name := c.resolveSenderName(ctx, info.Sender)
	m, ptr := normalizeDataMessage(info.Sender, name, msg.GetTimestamp(), msg, outgoing)
	m.MentionsMe = mentionsSelf(m.Mentions, c.selfID())
	resolveMentionNames(m.Mentions, c.nameFor)
	c.rememberAttachment(ctx, info.ChatID, m.RemoteID, ptr)

	// An edit carries neither a server GUID nor a report-spam token, so
	// only an incoming original is ever saved here; see handleEdit.
	if !outgoing {
		c.rememberMessageMeta(ctx, info.ChatID, m.RemoteID, info.ServerGUID, info.ReportingToken)
	}

	if seconds := msg.GetExpireTimer(); seconds > 0 {
		reportDisappearingTimer(ctx, sink, c.account.ID, info.ChatID, int64(seconds))
	}

	if poll := msg.GetPollCreate(); poll != nil {
		options := poll.GetOptions()
		c.trackPoll(ctx, info.ChatID, m.RemoteID, poll.GetQuestion(), options, poll.GetAllowMultiple())
	}

	if outgoing {
		sink.History(ctx, c.account.ID, info.ChatID, m)
		return
	}

	sink.Incoming(ctx, c.account.ID, info.ChatID, m)
}

// handleDelete reports a deleted message: only the message's own
// original sender can delete it, so info.Sender is also the target's
// author.
func (c *Connector) handleDelete(ctx context.Context, sink connector.Sink, info events.MessageInfo, del *signalpb.DataMessage_Delete) {
	remoteID := messageRemoteID(info.Sender, del.GetTargetSentTimestamp())
	sink.Deleted(ctx, c.account.ID, []string{info.ChatID}, []string{remoteID})
}

// handleReaction records a live reaction and reports the message's
// resulting set of chips.
func (c *Connector) handleReaction(ctx context.Context, sink connector.Sink, info events.MessageInfo, reaction *signalpb.DataMessage_Reaction) {
	targetAuthor, err := parseACI(reaction.GetTargetAuthorAci())
	if err != nil {
		return
	}

	targetRemoteID := messageRemoteID(targetAuthor, reaction.GetTargetSentTimestamp())
	emoji := reaction.GetEmoji()
	if reaction.GetRemove() {
		emoji = ""
	}

	key := info.ChatID + "/" + targetRemoteID
	chips := c.setReaction(key, info.Sender, emoji, c.selfID())
	sink.Reacted(ctx, c.account.ID, info.ChatID, targetRemoteID, chips)
}

// handlePollVote records a live vote and, when this run saw the poll
// created, reports its updated tallies.
func (c *Connector) handlePollVote(ctx context.Context, sink connector.Sink, info events.MessageInfo, vote *signalpb.DataMessage_PollVote) {
	targetAuthor, err := uuid.FromBytes(vote.GetTargetAuthorAciBinary())
	if err != nil {
		return
	}

	targetRemoteID := messageRemoteID(targetAuthor, vote.GetTargetSentTimestamp())
	indexes := make([]int, len(vote.GetOptionIndexes()))
	for i, idx := range vote.GetOptionIndexes() {
		indexes[i] = int(idx)
	}

	poll, ok := c.recordVote(ctx, info.ChatID, targetRemoteID, info.Sender, indexes, c.selfID())
	if !ok {
		return
	}

	c.reportPollUpdate(ctx, sink, info.ChatID, targetRemoteID, poll)
}

// handlePollTerminate records a poll closing and, when this run saw it
// created, reports its final tallies.
func (c *Connector) handlePollTerminate(ctx context.Context, sink connector.Sink, info events.MessageInfo, terminate *signalpb.DataMessage_PollTerminate) {
	targetRemoteID := messageRemoteID(info.Sender, terminate.GetTargetSentTimestamp())

	poll, ok := c.closePoll(ctx, info.ChatID, targetRemoteID, c.selfID())
	if !ok {
		return
	}

	c.reportPollUpdate(ctx, sink, info.ChatID, targetRemoteID, poll)
}

// handleTyping reports a typing indicator starting or stopping.
func (c *Connector) handleTyping(ctx context.Context, sink connector.Sink, info events.MessageInfo, typing *signalpb.TypingMessage) {
	name := c.resolveSenderName(ctx, info.Sender)
	active := typing.GetAction() == signalpb.TypingMessage_STARTED
	sink.Typing(ctx, c.account.ID, info.ChatID, name, active)
}

// handleEdit reports a message's text, formatting and media changing
// after it was sent.
func (c *Connector) handleEdit(ctx context.Context, sink connector.Sink, info events.MessageInfo, edit *signalpb.EditMessage) {
	data := edit.GetDataMessage()
	if data == nil {
		return
	}

	name := c.resolveSenderName(ctx, info.Sender)
	m, ptr := normalizeDataMessage(info.Sender, name, edit.GetTargetSentTimestamp(), data, false)
	m.Edited = true
	m.MentionsMe = mentionsSelf(m.Mentions, c.selfID())
	resolveMentionNames(m.Mentions, c.nameFor)
	c.rememberAttachment(ctx, info.ChatID, m.RemoteID, ptr)

	sink.Edited(ctx, c.account.ID, info.ChatID, m)
}

// handleContactList remembers every contact's resolved name.
func (c *Connector) handleContactList(list *events.ContactList) {
	for _, r := range list.Contacts {
		if r == nil || r.ACI == uuid.Nil {
			continue
		}

		if name := contactDisplayName(r); name != "" {
			c.rememberName(senderRemoteID(r.ACI), name)
		}
	}
}

// contactDisplayName picks the best name a recipient record carries:
// a saved contact name over a profile name.
func contactDisplayName(r *types.Recipient) string {
	if r.ContactName != "" {
		return r.ContactName
	}

	return r.Profile.Name
}

// handleACIFound upgrades a contact first seen by phone number (PNI)
// to its real account identifier (ACI), merging the conversation.
func (c *Connector) handleACIFound(ctx context.Context, sink connector.Sink, found *events.ACIFound) {
	upgrader, ok := sink.(connector.RemoteIDUpgrader)
	if !ok {
		return
	}

	oldRemoteID := found.PNI.UUID.String()
	newRemoteID := found.ACI.UUID.String()
	upgrader.ConversationRemoteIDChanged(ctx, c.account.ID, oldRemoteID, newRemoteID)
}

// ensureConversation reports info's conversation the first time this
// run sees it, resolving a group's title and members from Signal.
func (c *Connector) ensureConversation(ctx context.Context, sink connector.Sink, info events.MessageInfo) {
	if c.markReported(info.ChatID) {
		return
	}

	if !isGroupChat(info.ChatID) {
		sink.Conversation(ctx, domain.Conversation{
			AccountID: c.account.ID, Service: domain.ServiceSignal,
			RemoteID: info.ChatID, Kind: domain.KindDirect,
		})
		return
	}

	c.reportGroup(ctx, sink, types.GroupIdentifier(info.ChatID))
}

// reportGroup fetches and reports a group's current title, member
// count and disappearing timer.
func (c *Connector) reportGroup(ctx context.Context, sink connector.Sink, gid types.GroupIdentifier) {
	dev, _, err := c.session()
	if err != nil {
		return
	}

	title, members, seconds, err := dev.groupInfo(ctx, gid)
	if err != nil {
		return
	}

	c.rememberName(string(gid), title)
	sink.Conversation(ctx, domain.Conversation{
		AccountID: c.account.ID, Service: domain.ServiceSignal,
		RemoteID: string(gid), Kind: domain.KindGroup,
		Title: title, Members: len(members),
	})
	reportDisappearingTimer(ctx, sink, c.account.ID, string(gid), seconds)
}

// resolveSenderName returns the best name known for aci, asking the
// device when nothing is cached yet, and falls back to
// genericSenderName when nothing resolves.
func (c *Connector) resolveSenderName(ctx context.Context, aci uuid.UUID) string {
	remoteID := senderRemoteID(aci)
	if name := c.nameFor(remoteID); name != "" {
		return name
	}

	dev, _, err := c.session()
	if err != nil {
		return genericSenderName
	}

	name, found := dev.contactName(ctx, aci)
	if !found || name == "" {
		return genericSenderName
	}

	c.rememberName(remoteID, name)

	return name
}

// mentionsSelf reports whether mentions names self.
func mentionsSelf(mentions []domain.Mention, self uuid.UUID) bool {
	if self == uuid.Nil {
		return false
	}

	selfID := senderRemoteID(self)
	for _, m := range mentions {
		if m.UserID == selfID {
			return true
		}
	}

	return false
}

// resolveMentionNames fills in each mention's display name from
// nameFor, falling back to genericSenderName for one not yet resolved.
func resolveMentionNames(mentions []domain.Mention, nameFor func(string) string) {
	for i, m := range mentions {
		if name := nameFor(m.UserID); name != "" {
			mentions[i].Name = name
		} else {
			mentions[i].Name = genericSenderName
		}
	}
}

// reportDisappearingTimer reports seconds as conversationRemoteID's
// disappearing-message period, when sink supports it.
func reportDisappearingTimer(ctx context.Context, sink connector.Sink, accountID, conversationRemoteID string, seconds int64) {
	reporter, ok := sink.(connector.DisappearingTimerReporter)
	if !ok {
		return
	}

	reporter.DisappearingTimer(ctx, accountID, conversationRemoteID, seconds)
}

// rememberMessageMeta saves an incoming message's server GUID and
// report-spam token, for a later spam report. Best effort, the same
// as rememberAttachment: without a media store nothing is saved.
func (c *Connector) rememberMessageMeta(ctx context.Context, conversationRemoteID, messageRemoteID, serverGUID string, reportToken []byte) {
	media := c.mediaFor()
	if media == nil {
		return
	}

	_ = media.putMessageMeta(ctx, conversationRemoteID, messageRemoteID, serverGUID, reportToken)
}

// reportPollUpdate reports poll's updated tallies, when sink supports
// it.
func (c *Connector) reportPollUpdate(ctx context.Context, sink connector.Sink, conversationRemoteID, messageRemoteID string, poll domain.Poll) {
	updater, ok := sink.(connector.PollUpdater)
	if !ok {
		return
	}

	updater.PollUpdated(ctx, c.account.ID, conversationRemoteID, messageRemoteID, poll)
}
