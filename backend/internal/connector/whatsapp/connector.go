// Package whatsapp connects a WhatsApp account through whatsmeow: it
// pairs by QR code or a phone number's link code, syncs history, follows
// live messages, receipts, typing and organizing changes, and normalizes
// whatsmeow's JIDs, messages and sync data into the domain types the rest
// of the helper uses. A later wave adds sending and outgoing receipts.
package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// ErrAlreadyRunning reports a second concurrent Run on the same account.
var ErrAlreadyRunning = errors.New("whatsapp: already running")

// errNotPairing reports sign-in input when no pairing is waiting for it.
var errNotPairing = errors.New("whatsapp: not waiting to pair")

// errSendNotSupported reports Send and MarkRead, which a later wave adds.
var errSendNotSupported = errors.New("whatsapp: sending is not supported yet")

// organizeState is the pinned and archived flags this connector last
// knew for one conversation, kept so a live Pin or Archive event, which
// each report only one of the two, can still call Sink.Organized with
// both: it is seeded from history sync and updated by those events for
// as long as this process runs.
type organizeState struct {
	pinned, archived bool
}

// Connector is one WhatsApp account.
type Connector struct {
	account   domain.Account
	answers   chan answer
	open      func(ctx context.Context) (device, error)
	openMedia func(ctx context.Context) (*mediaStore, error)

	mu        sync.Mutex
	running   bool
	waiting   bool
	dev       device
	organize  map[string]organizeState     // conversation remote id to its last known pinned/archived state
	names     map[string]string            // contact, push and group names resolved so far, by remote id
	reactions map[string]map[string]string // "<conversation remote id>/<message remote id>" to who reacted and with which emoji
}

var (
	_ connector.Connector      = (*Connector)(nil)
	_ connector.Authenticator  = (*Connector)(nil)
	_ connector.LogoutOnRemove = (*Connector)(nil)
)

// New returns the connector for an account whose session is kept in dir.
func New(account domain.Account, dir string) *Connector {
	return &Connector{
		account:   account,
		answers:   make(chan answer, 1),
		open:      func(ctx context.Context) (device, error) { return openDevice(ctx, dir, account.ID) },
		openMedia: func(ctx context.Context) (*mediaStore, error) { return openMediaStore(ctx, dir, account.ID) },
		organize:  map[string]organizeState{},
		names:     map[string]string{},
		reactions: map[string]map[string]string{},
	}
}

// Account describes the WhatsApp account.
func (c *Connector) Account() domain.Account {
	return c.account
}

// Run opens the account's session, pairs it if it is not already paired,
// and then reports its connection status, history, live messages and
// organizing changes until ctx is cancelled or the connection ends for
// good.
func (c *Connector) Run(ctx context.Context, sink connector.Sink) error {
	if err := c.startRun(); err != nil {
		return err
	}
	defer c.endRun()

	dev, err := c.open(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: open session: %w", err)
	}
	defer func() { _ = dev.close() }() // the connection below is what matters; a close failure changes nothing

	media, err := c.openMedia(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: open media store: %w", err)
	}
	defer func() { _ = media.close() }() // same as the session close above

	c.setDevice(dev)
	defer c.setDevice(nil)

	sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")

	stopped := make(chan error, 1)
	unregister := dev.onStatus(func(status string) { c.reportStatus(ctx, sink, status, stopped) })
	defer unregister()

	unregisterEvents := dev.onEvent(func(evt any) { c.dispatch(ctx, sink, dev, media, evt) })
	defer unregisterEvents()

	if err := c.connectOrPair(ctx, dev, sink); err != nil {
		return err
	}
	defer dev.disconnect()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-stopped:
		return err
	}
}

// Logout tells WhatsApp to unlink this device, if Run has one connected;
// Manager.Remove calls it, best effort, before stopping this connector
// for good.
func (c *Connector) Logout(ctx context.Context) error {
	c.mu.Lock()
	dev := c.dev
	c.mu.Unlock()

	if dev == nil {
		return nil
	}

	return dev.logOut(ctx)
}

// setDevice records the device the running connection uses, so Logout can
// reach it, or clears it once Run returns.
func (c *Connector) setDevice(dev device) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev = dev
}

// connectOrPair connects a session that is already paired, or pairs a
// new one first, reporting that the account needs attention while it
// waits for the user.
func (c *Connector) connectOrPair(ctx context.Context, dev device, sink connector.Sink) error {
	if dev.isPaired() {
		if err := dev.connect(ctx); err != nil {
			return fmt.Errorf("whatsapp: connect: %w", err)
		}

		return nil
	}

	sink.AccountStatus(ctx, c.account.ID, domain.AccountNeedsAuth, "")
	c.setWaiting(true)
	defer c.setWaiting(false)

	report := func(step connector.AuthStep) { sink.AuthStep(ctx, c.account.ID, step) }

	return pair(ctx, dev, c.answers, report)
}

// reportStatus turns a device status into the account status the sink
// shows, or, for a disconnect whatsmeow will not recover from on its
// own, ends Run so the Manager restarts it and tries pairing again.
func (c *Connector) reportStatus(ctx context.Context, sink connector.Sink, status string, stopped chan<- error) {
	switch status {
	case statusConnected:
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnected, "")
	case statusDisconnected:
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")
	case statusStopped:
		select {
		case stopped <- errors.New("whatsapp: disconnected and will not reconnect on its own"):
		default:
		}
	}
}

// SubmitAuth answers the step pairing is waiting for.
func (c *Connector) SubmitAuth(ctx context.Context, step, value string) error {
	c.mu.Lock()
	waiting := c.waiting
	c.mu.Unlock()

	if !waiting {
		return errNotPairing
	}

	select {
	case c.answers <- answer{step: step, value: value}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Send is not supported yet; a later wave adds it.
func (*Connector) Send(_ context.Context, _ domain.Conversation, _ domain.Message) error {
	return errSendNotSupported
}

// MarkRead is not supported yet; a later wave adds it.
func (*Connector) MarkRead(_ context.Context, _ domain.Conversation) error {
	return errSendNotSupported
}

// startRun records that this connector is running, refusing a second
// concurrent Run.
func (c *Connector) startRun() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return ErrAlreadyRunning
	}

	c.running = true

	return nil
}

// endRun records that Run has returned.
func (c *Connector) endRun() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.running = false
}

// setWaiting records whether pairing is waiting for the user.
func (c *Connector) setWaiting(waiting bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.waiting = waiting
}

// nameFor returns the best name known for remoteID, a contact, push or
// group name resolved so far, or "" when nothing has resolved one yet.
func (c *Connector) nameFor(remoteID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.names[remoteID]
}

// setName records the resolved name for remoteID, so later messages,
// conversations and presence updates for it do not have to resolve it
// again.
func (c *Connector) setName(remoteID, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.names[remoteID] = name
}

// setOrganized merges a change into remoteID's last known pinned and
// archived state, leaving whichever of the two is nil as it was, and
// returns the merged result.
func (c *Connector) setOrganized(remoteID string, pinned, archived *bool) organizeState {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.organize[remoteID]
	if pinned != nil {
		state.pinned = *pinned
	}
	if archived != nil {
		state.archived = *archived
	}
	c.organize[remoteID] = state

	return state
}

// reactTo records sender's reaction to a message as emoji, or clears it
// when emoji is "", and returns the message's full, recomputed tally.
func (c *Connector) reactTo(conversationRemoteID, messageRemoteID, sender, emoji string) []domain.Reaction {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := conversationRemoteID + "/" + messageRemoteID
	bySender := c.reactions[key]
	if bySender == nil {
		bySender = map[string]string{}
		c.reactions[key] = bySender
	}

	if emoji == "" {
		delete(bySender, sender)
	} else {
		bySender[sender] = emoji
	}

	return reactionTally(bySender, "self")
}
