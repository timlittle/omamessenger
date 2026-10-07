package whatsapp

// receipts.go reports delivery and read progress for messages this
// account sent, from the receipts whatsmeow reports for them, and tells
// WhatsApp when the user has read a conversation.
//
// For a group, WhatsApp's own tick only advances once every member has
// caught up: the double tick turns up once everyone's device has the
// message, and blue once everyone has opened it. This file matches that
// rather than reporting on the first receipt: a group message's
// participants are tracked until as many have reported as
// expectedRecipients (see send.go) expects, and the status reported is
// the slowest participant's, not the fastest one's. Reporting on the
// first receipt instead would show a group chat as "read" the moment
// one of many members opened it, which is not what WhatsApp shows.

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// maxPendingReadIDsPerSender bounds how many unread message ids this
// connector remembers for one sender in one conversation, so a chat
// that is never marked read cannot grow this connector's memory without
// bound.
const maxPendingReadIDsPerSender = 500

// readTimeout bounds how long MarkRead waits for WhatsApp to accept a
// read receipt.
const readTimeout = 30 * time.Second

// rankOf orders the receipt statuses this connector acts on, so a
// group's slowest participant can be found by comparing ranks; 0 means a
// status nothing here tracks.
func rankOf(status string) int {
	switch status {
	case domain.StatusDelivered:
		return 1
	case domain.StatusRead:
		return 2
	default:
		return 0
	}
}

// sentMessage is a message this account sent, kept so a receipt
// reporting WhatsApp's id for it can be matched back to the local
// message, and, for a group, so its tick advances only once every
// expected participant has reached a given status.
type sentMessage struct {
	localID  string
	expected int
	reached  map[string]int
}

// sentKey identifies a sent message by its chat and WhatsApp id, the two
// a receipt always carries.
func sentKey(chatRemoteID, remoteMsgID string) string {
	return chatRemoteID + "/" + remoteMsgID
}

// trackSent remembers a message this account just sent under key (see
// sentKey), so a later receipt naming the same chat and WhatsApp id can
// find the local message it reports progress for.
func (c *Connector) trackSent(key, localID string, expected int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sent == nil {
		c.sent = map[string]*sentMessage{}
	}
	c.sent[key] = &sentMessage{localID: localID, expected: expected, reached: map[string]int{}}
}

// receipt reports delivery or read progress for the messages a
// whatsmeow receipt event names, dropping any id this account did not
// send or whose progress whatsmeow reports in a kind this connector does
// not track, such as a retry request.
func (c *Connector) receipt(ctx context.Context, sink connector.Sink, e *events.Receipt) {
	rank := rankOf(receiptStatus(e.Type))
	if rank == 0 {
		return
	}

	chat, participant := remoteID(e.Chat), remoteID(e.Sender)
	for _, id := range e.MessageIDs {
		if localID, status := c.advanceSent(sentKey(chat, id), participant, rank); status != "" {
			sink.OutgoingStatus(ctx, localID, "", status)
		}
	}
}

// advanceSent records that participant has reached rank for the sent
// message named by key, and reports its new status once every expected
// participant has reached at least that far, the slowest one deciding
// the status reported. It returns "" when the message is not one this
// account tracked, or when fewer participants than expected have
// reported anything yet.
func (c *Connector) advanceSent(key, participant string, rank int) (localID, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	sm, ok := c.sent[key]
	if !ok {
		return "", ""
	}

	if rank > sm.reached[participant] {
		sm.reached[participant] = rank
	}

	status, complete := completedStatus(sm)
	if !complete {
		return "", ""
	}

	if status == domain.StatusRead {
		delete(c.sent, key) // nothing further to track once everyone has read it
	}

	return sm.localID, status
}

// completedStatus is the status every expected participant has reached,
// the slowest one deciding it, or "" when fewer participants than
// expected have reported anything yet.
func completedStatus(sm *sentMessage) (status string, complete bool) {
	if len(sm.reached) < sm.expected {
		return "", false
	}

	min := rankOf(domain.StatusRead)
	for _, r := range sm.reached {
		if r < min {
			min = r
		}
	}

	switch min {
	case rankOf(domain.StatusRead):
		return domain.StatusRead, true
	case rankOf(domain.StatusDelivered):
		return domain.StatusDelivered, true
	default:
		return "", false
	}
}

// notePendingRead records an incoming message as unread, so a later
// MarkRead for its conversation tells WhatsApp it was read. The live
// message handler calls this for every message it reports through the
// sink's Incoming, except this account's own messages from another
// device, which are never unread.
func (c *Connector) notePendingRead(convRemoteID, senderRemoteID, messageRemoteID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.unread == nil {
		c.unread = map[string]map[string][]string{}
	}
	bySender := c.unread[convRemoteID]
	if bySender == nil {
		bySender = map[string][]string{}
		c.unread[convRemoteID] = bySender
	}

	ids := bySender[senderRemoteID]
	if len(ids) >= maxPendingReadIDsPerSender {
		ids = ids[1:] // drop the oldest so a chat never marked read cannot grow this without bound
	}
	bySender[senderRemoteID] = append(ids, messageRemoteID)
}

// takePendingRead returns and forgets the unread message ids tracked for
// the conversation named by convRemoteID, by sender, so MarkRead sends
// each sender's receipt once.
func (c *Connector) takePendingRead(convRemoteID string) map[string][]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	pending := c.unread[convRemoteID]
	delete(c.unread, convRemoteID)

	return pending
}

// MarkRead tells WhatsApp the unread messages tracked for conv have been
// read, one call per sender since WhatsApp only accepts message ids from
// a single sender in one call; in a group, that sender is the
// participant who sent them.
func (c *Connector) MarkRead(ctx context.Context, conv domain.Conversation) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	pending := c.takePendingRead(conv.RemoteID)
	if len(pending) == 0 {
		return nil
	}

	chat, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: mark read: %w", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	for senderRemoteID, ids := range pending {
		if err := markReadFrom(readCtx, dev, chat, senderRemoteID, ids); err != nil {
			return err
		}
	}

	return nil
}

// markReadFrom tells WhatsApp the messages ids, all sent by the
// participant named senderRemoteID, have been read, skipping a sender id
// this connector did not make rather than failing the whole call.
func markReadFrom(ctx context.Context, dev device, chat types.JID, senderRemoteID string, ids []string) error {
	sender, err := jidFromRemoteID(senderRemoteID)
	if err != nil {
		return nil
	}

	messageIDs := make([]types.MessageID, len(ids))
	for i, id := range ids {
		messageIDs[i] = types.MessageID(id)
	}

	if err := dev.markRead(ctx, messageIDs, chat, sender); err != nil {
		return fmt.Errorf("whatsapp: mark read: %w", err)
	}

	return nil
}
