package whatsapp

// history_ondemand.go implements LoadOlder (connector.HistoryLoader) for
// WhatsApp: asking the account's own primary phone to backfill a chat
// past what history sync has already delivered, the same mechanism
// mautrix-whatsapp uses. whatsmeow's BuildHistorySyncRequest builds the
// request, but unlike an ordinary RPC it is not a request/response call:
// it has to be sent to the primary phone as an ordinary peer message
// (requestOlderHistory in device.go), and the phone answers later,
// asynchronously and with nothing that names which request it is
// answering, as another events.HistorySync of type ON_DEMAND, handled
// the normal way history.go always handles a sync. This file keeps the
// table that matches that answer back to the LoadOlder call waiting for
// it, by the chat it named, and times the wait out if the phone never
// answers at all.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// onDemandHistoryTimeout bounds how long LoadOlder waits for the primary
// phone to answer an on-demand history request, so a phone that is
// offline or never answers cannot leave a scroll-back waiting forever.
const onDemandHistoryTimeout = 30 * time.Second

// errHistoryRequestInProgress reports a second LoadOlder call for a
// conversation that already has an on-demand history request waiting on
// the primary phone's answer.
var errHistoryRequestInProgress = errors.New("whatsapp: an older history request for this conversation is already waiting on the phone")

// LoadOlder asks WhatsApp's primary phone for up to limit messages
// older than the one message_keys last recorded for beforeRemoteID (see
// keys.go), reporting them through the Sink's History the way an
// ordinary history sync always does (see history.go), and says how many
// arrived. Nothing is asked, and 0, nil is returned, when this run has
// no session, no media store (message_keys lives there) or no anchor to
// ask from: beforeRemoteID is "" (nothing stored yet to anchor from) or
// message_keys has no row for it (a message stored by something other
// than this connector's own history or live path, which has never
// happened in practice). A phone that never answers within
// onDemandHistoryTimeout is reported as connector.ErrHistoryUnavailable,
// distinct from an answer that simply carried no more messages, which
// this reports as 0, nil, the same as "no older history".
func (c *Connector) LoadOlder(ctx context.Context, conv domain.Conversation, beforeRemoteID string, limit int) (int, error) {
	dev, _, err := c.session()
	if err != nil {
		return 0, err
	}

	media := c.mediaFor()
	if media == nil || beforeRemoteID == "" {
		return 0, nil
	}

	chat, err := jidFromRemoteID(conv.RemoteID)
	if err != nil {
		return 0, fmt.Errorf("whatsapp: older history: %w", err)
	}

	key, found, err := media.messageKeyFor(ctx, conv.RemoteID, beforeRemoteID)
	if err != nil {
		return 0, fmt.Errorf("whatsapp: older history: %w", err)
	}
	if !found {
		return 0, nil
	}

	anchor := historyAnchor(chat, beforeRemoteID, key)

	return c.requestOlderHistory(ctx, dev, conv.RemoteID, anchor, limit)
}

// historyAnchor builds the message identity BuildHistorySyncRequest
// needs to ask for history before it: the chat, the message's id,
// whether this account sent it, and when, from whichever of those
// message_keys last recorded for it (see keys.go). The sender itself is
// not part of the request whatsmeow builds, so unlike targetKey this
// never needs one.
func historyAnchor(chat types.JID, messageRemoteID string, key messageKey) *types.MessageInfo {
	return &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, IsFromMe: key.fromMe},
		ID:            types.MessageID(messageRemoteID),
		Timestamp:     time.UnixMilli(key.timestamp),
	}
}

// requestOlderHistory sends chatRemoteID's on-demand history request
// through dev and waits for the primary phone's answer, resolved by
// handleHistorySync once it arrives as an events.HistorySync of type
// ON_DEMAND for the same chat (see deliverOnDemandHistory). Only one
// such wait is ever registered per chat at a time.
func (c *Connector) requestOlderHistory(ctx context.Context, dev device, chatRemoteID string, anchor *types.MessageInfo, limit int) (int, error) {
	ch, cleanup, err := c.registerOnDemandWaiter(chatRemoteID)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	if err := dev.requestOlderHistory(ctx, anchor, limit); err != nil {
		return 0, fmt.Errorf("whatsapp: older history: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, onDemandHistoryTimeout)
	defer cancel()

	select {
	case count := <-ch:
		return count, nil
	case <-waitCtx.Done():
		return 0, connector.ErrHistoryUnavailable
	}
}

// registerOnDemandWaiter reserves chatRemoteID's slot in this
// connector's table of pending on-demand history requests, refusing a
// second one for the same chat while the first is still waiting (see
// errHistoryRequestInProgress) rather than letting two pile up behind
// the same slot, unlike registerRetryWaiter (retry.go), which
// overwrites. The returned cleanup must run once the caller stops
// waiting, successfully or not, so a request nobody is listening for
// any more cannot block every later LoadOlder for the same chat.
func (c *Connector) registerOnDemandWaiter(chatRemoteID string) (<-chan int, func(), error) {
	ch, ok := c.onDemandWaiters.register(chatRemoteID, true)
	if !ok {
		return nil, nil, errHistoryRequestInProgress
	}

	return ch, func() { c.onDemandWaiters.cleanup(chatRemoteID) }, nil
}

// deliverOnDemandHistory hands an on-demand history sync's reported
// message count to whichever LoadOlder call is waiting for chatRemoteID,
// or drops it when nothing is waiting: an on-demand sync for a chat this
// run never asked one for, or one that already timed out and stopped
// listening.
func (c *Connector) deliverOnDemandHistory(chatRemoteID string, count int) {
	c.onDemandWaiters.deliver(chatRemoteID, count)
}
