// Package whatsapp connects a WhatsApp account through whatsmeow: it
// pairs by QR code or a phone number's link code, syncs history, follows
// live messages, receipts, typing and organizing changes, sends and
// downloads photos, videos and files alongside outgoing text, reports
// delivery and read progress, and normalizes whatsmeow's JIDs, messages
// and sync data into the domain types the rest of the helper uses.
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

// errNotConnected reports Send or MarkRead asked of an account whose Run
// is not currently connected.
var errNotConnected = errors.New("whatsapp: not connected")

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

	mu      sync.Mutex
	running bool
	waiting bool
	dev     device
	sink    connector.Sink
	media   *mediaStore // this run's media reference store; see setMedia

	// sent and unread are read and written by send.go and receipts.go:
	// sent matches a receipt's chat and WhatsApp id back to the local
	// message it reports progress for, and unread tracks incoming
	// message ids MarkRead has not yet told WhatsApp about.
	sent   map[string]*sentMessage
	unread map[string]map[string][]string

	organize  map[string]organizeState     // conversation remote id to its last known pinned/archived state
	names     map[string]string            // contact, push and group names resolved so far, by remote id
	reactions map[string]map[string]string // "<conversation remote id>/<message remote id>" to who reacted and with which emoji
}

var (
	_ connector.Connector      = (*Connector)(nil)
	_ connector.Authenticator  = (*Connector)(nil)
	_ connector.LogoutOnRemove = (*Connector)(nil)
	_ connector.MediaFetcher   = (*Connector)(nil)
)

// New returns the connector for an account whose session is kept in
// dir, having identified this helper to WhatsApp's own device list (see
// identity.go) before Run can start pairing it.
func New(account domain.Account, dir string) *Connector {
	identifyDevice()

	return &Connector{
		account:   account,
		answers:   make(chan answer, 1),
		open:      func(ctx context.Context) (device, error) { return openDevice(ctx, dir, account.ID) },
		openMedia: func(ctx context.Context) (*mediaStore, error) { return openMediaStore(ctx, dir, account.ID) },
	}
}

// Account describes the WhatsApp account.
func (c *Connector) Account() domain.Account {
	return c.account
}

// Run opens the account's session, pairs it if it is not already paired,
// and then reports its connection status, history, live messages and
// organizing changes, and sends and tracks outgoing messages, until ctx
// is cancelled or the connection ends for good.
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

	c.setMedia(media)
	defer c.clearMedia()

	sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")

	stopped := make(chan error, 1)
	unregister := dev.onStatus(func(status string) { c.reportStatus(ctx, sink, status, stopped) })
	defer unregister()

	if err := c.connectOrPair(ctx, dev, sink); err != nil {
		return err
	}
	defer dev.disconnect()

	c.connected(dev, sink)
	defer c.disconnected()

	unregisterEvents := c.handleEvents(ctx, dev, media, sink)
	defer unregisterEvents()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-stopped:
		return err
	}
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

// Logout tells WhatsApp to unlink this device, if a run is currently
// connected. Manager.Remove calls it, best effort and bounded, before
// stopping this connector for good; when nothing is connected there is
// nothing to unlink, so it reports success rather than errNotConnected.
func (c *Connector) Logout(ctx context.Context) error {
	dev, _, err := c.session()
	if err != nil {
		return nil
	}

	return dev.logOut(ctx)
}

// connected records the device and sink of a signed-in run, so Send,
// MarkRead and incoming events have something to act on.
func (c *Connector) connected(dev device, sink connector.Sink) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev, c.sink = dev, sink
}

// disconnected forgets the run's device and sink, so Send and MarkRead
// fail until the account reconnects.
func (c *Connector) disconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev, c.sink = nil, nil
}

// session returns the device and sink of the current run, or
// errNotConnected when no run is connected.
func (c *Connector) session() (device, connector.Sink, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dev == nil {
		return nil, nil, errNotConnected
	}

	return c.dev, c.sink, nil
}

// setMedia records this run's media reference store, for FetchMedia and
// outgoing sends to save and look up references in.
func (c *Connector) setMedia(media *mediaStore) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.media = media
}

// clearMedia forgets the run's media reference store, so FetchMedia and
// outgoing sends stop using it once Run ends.
func (c *Connector) clearMedia() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.media = nil
}

// currentMedia returns the media reference store of the current run,
// or errNotConnected when no run has one open.
func (c *Connector) currentMedia() (*mediaStore, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.media == nil {
		return nil, errNotConnected
	}

	return c.media, nil
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

	if c.names == nil {
		c.names = map[string]string{}
	}
	c.names[remoteID] = name
}

// setOrganized merges a change into remoteID's last known pinned and
// archived state, leaving whichever of the two is nil as it was, and
// returns the merged result.
func (c *Connector) setOrganized(remoteID string, pinned, archived *bool) organizeState {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.organize == nil {
		c.organize = map[string]organizeState{}
	}

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

	if c.reactions == nil {
		c.reactions = map[string]map[string]string{}
	}

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
