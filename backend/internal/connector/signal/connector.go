package signal

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// ErrAlreadyRunning reports a second concurrent Run on the same account.
var ErrAlreadyRunning = errors.New("signal: already running")

// errNotConnected reports an action asked of an account whose Run is
// not currently connected.
var errNotConnected = errors.New("signal: not connected")

// errNotImplemented reports a FetchMedia asked for a message this
// connector never saved an attachment pointer for.
var errNotImplemented = errors.New("signal: not yet implemented")

// opener opens the account's session, linking by QR code if needed,
// and reports whether this call just completed a fresh link.
type opener func(ctx context.Context, handler func(events.SignalEvent) bool, report func(connector.AuthStep)) (dev device, freshlyLinked bool, err error)

// Connector is one Signal account.
type Connector struct {
	account   domain.Account
	open      opener
	openMedia func(ctx context.Context) (*mediaStore, error)

	mu      sync.Mutex
	running bool
	dev     device
	sink    connector.Sink
	media   *mediaStore
	selfACI uuid.UUID

	identityMu sync.Mutex
	names      map[string]string // resolved contact and group names, by remote id

	// reactions tracks who reacted to each message; see reaction.go.
	// In memory only, not persisted, matching the WhatsApp connector.
	reactions     map[string]map[uuid.UUID]string
	reactionOrder []string
	reportedChats map[string]bool // conversation remote ids already reported this run

	// chatIDs maps a conversation's remote id to the transferred
	// history's own chat id, for LoadOlder (see history.go).
	chatIDs map[string]uint64
}

var (
	_ connector.Connector      = (*Connector)(nil)
	_ connector.HistoryLoader  = (*Connector)(nil)
	_ connector.MemberLister   = (*Connector)(nil)
	_ connector.MediaFetcher   = (*Connector)(nil)
	_ connector.LogoutOnRemove = (*Connector)(nil)
)

// New returns the connector for an account whose session is kept in dir.
func New(account domain.Account, dir string) *Connector {
	c := &Connector{account: account}
	c.open = func(ctx context.Context, handler func(events.SignalEvent) bool, report func(connector.AuthStep)) (device, bool, error) {
		return openDevice(ctx, dir, account.ID, linkDeps{handler: handler, report: report, provision: signalmeow.PerformProvisioning})
	}
	c.openMedia = func(ctx context.Context) (*mediaStore, error) { return openMediaStore(ctx, dir, account.ID) }

	return c
}

// Account describes the Signal account.
func (c *Connector) Account() domain.Account {
	return c.account
}

// Run opens the account's session, links it by QR code if needed,
// imports transferred history on a fresh link, and follows live
// messages until ctx is cancelled or the connection ends for good.
func (c *Connector) Run(ctx context.Context, sink connector.Sink) error {
	if err := c.startRun(); err != nil {
		return err
	}
	defer c.endRun()

	sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")

	report := func(step connector.AuthStep) { sink.AuthStep(ctx, c.account.ID, step) }
	handler := func(evt events.SignalEvent) bool { return c.handleEvent(ctx, sink, evt) }

	dev, freshlyLinked, err := c.open(ctx, handler, report)
	if err != nil {
		return fmt.Errorf("signal: open: %w", err)
	}
	defer func() { _ = dev.close() }() // the receive loop below is what matters; a close failure changes nothing

	media, err := c.openMedia(ctx)
	if err != nil {
		return fmt.Errorf("signal: open media store: %w", err)
	}
	defer func() { _ = media.close() }() // same as the session close above

	statusCh, err := dev.startReceiveLoops(ctx)
	if err != nil {
		return fmt.Errorf("signal: connect: %w", err)
	}
	defer func() { _ = dev.stopReceiveLoops() }()

	c.connected(dev, sink, media)
	defer c.disconnected()

	return c.watch(ctx, dev, sink, statusCh, freshlyLinked)
}

// watch runs the history import alongside Run's wait for ctx to be
// cancelled or the connection to end for good, and cancels the import
// first so it never outlives dev.
func (c *Connector) watch(ctx context.Context, dev device, sink connector.Sink, statusCh <-chan signalmeow.SignalConnectionStatus, freshlyLinked bool) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	if freshlyLinked {
		wg.Go(func() { c.importHistory(runCtx, dev, sink) })
	}
	defer wg.Wait()

	for {
		select {
		case status, ok := <-statusCh:
			if !ok {
				return errors.New("signal: receive loop ended unexpectedly")
			}

			if terminal, err := c.reportStatus(ctx, sink, status); terminal {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// reportStatus turns a signalmeow connection status into the account
// status the sink shows, or reports that Run should end for good.
func (c *Connector) reportStatus(ctx context.Context, sink connector.Sink, status signalmeow.SignalConnectionStatus) (terminal bool, err error) {
	switch status.Event {
	case signalmeow.SignalConnectionEventConnected:
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnected, "")
	case signalmeow.SignalConnectionEventDisconnected, signalmeow.SignalConnectionEventError:
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")
	case signalmeow.SignalConnectionEventLoggedOut:
		return true, fmt.Errorf("signal: logged out: %w", status.Err)
	case signalmeow.SignalConnectionEventFatalError:
		return true, fmt.Errorf("signal: connection failed: %w", status.Err)
	}

	return false, nil
}

// Logout tells Signal to unlink this device when a run is connected,
// and reports success when nothing is connected: nothing to unlink.
func (c *Connector) Logout(ctx context.Context) error {
	dev, _, err := c.session()
	if err != nil {
		return nil //nolint:nilerr // deliberate: see the doc comment above, nothing connected means nothing to unlink
	}

	return dev.unlink(ctx)
}

// connected records the device, sink and media store of a signed-in
// run, so Logout, LoadOlder, Members, FetchMedia and incoming events
// have something to act on.
func (c *Connector) connected(dev device, sink connector.Sink, media *mediaStore) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev, c.sink, c.media, c.selfACI = dev, sink, media, dev.aci()
}

// selfID returns this run's own account identifier, or uuid.Nil
// before any run has connected.
func (c *Connector) selfID() uuid.UUID {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.selfACI
}

// disconnected forgets the run's device, sink and media store.
func (c *Connector) disconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev, c.sink, c.media = nil, nil, nil
}

// mediaFor returns the media store of the current run, or nil when
// this connector was never given one, such as a test built without one.
func (c *Connector) mediaFor() *mediaStore {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.media
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

// rememberName records the best name resolved so far for a contact or
// group's remote id.
func (c *Connector) rememberName(remoteID, name string) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	if c.names == nil {
		c.names = map[string]string{}
	}
	c.names[remoteID] = name
}

// nameFor returns the best name resolved so far for remoteID, or "" if
// none has resolved yet.
func (c *Connector) nameFor(remoteID string) string {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	return c.names[remoteID]
}

// markReported records that conversationRemoteID has been reported
// this run, and reports whether it already had been.
func (c *Connector) markReported(conversationRemoteID string) (alreadyReported bool) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	if c.reportedChats == nil {
		c.reportedChats = map[string]bool{}
	}

	alreadyReported = c.reportedChats[conversationRemoteID]
	c.reportedChats[conversationRemoteID] = true

	return alreadyReported
}

// forgetReported lets the next message report conversationRemoteID
// again, after a report made with incomplete details.
func (c *Connector) forgetReported(conversationRemoteID string) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	delete(c.reportedChats, conversationRemoteID)
}
