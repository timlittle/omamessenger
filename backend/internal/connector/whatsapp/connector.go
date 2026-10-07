// Package whatsapp connects a WhatsApp account through whatsmeow: it
// pairs by QR code or a phone number's link code, reports the account's
// connection status to a Sink, and normalizes whatsmeow's JIDs, messages
// and sync data into the domain types the rest of the helper uses. It
// sends outgoing text and reports delivery and read progress; a later
// wave adds sending media.
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

// Connector is one WhatsApp account.
type Connector struct {
	account domain.Account
	answers chan answer
	open    func(ctx context.Context) (device, error)

	mu      sync.Mutex
	running bool
	waiting bool
	dev     device
	sink    connector.Sink

	// sent and unread are read and written by send.go and receipts.go:
	// sent matches a receipt's chat and WhatsApp id back to the local
	// message it reports progress for, and unread tracks incoming
	// message ids MarkRead has not yet told WhatsApp about.
	sent   map[string]*sentMessage
	unread map[string]map[string][]string
}

var (
	_ connector.Connector     = (*Connector)(nil)
	_ connector.Authenticator = (*Connector)(nil)
)

// New returns the connector for an account whose session is kept in dir.
func New(account domain.Account, dir string) *Connector {
	return &Connector{
		account: account,
		answers: make(chan answer, 1),
		open:    func(ctx context.Context) (device, error) { return openDevice(ctx, dir, account.ID) },
	}
}

// Account describes the WhatsApp account.
func (c *Connector) Account() domain.Account {
	return c.account
}

// Run opens the account's session, pairs it if it is not already paired,
// and then reports its connection status until ctx is cancelled or the
// connection ends for good.
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

	unregisterEvents := c.handleEvents(ctx, dev, sink)
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
