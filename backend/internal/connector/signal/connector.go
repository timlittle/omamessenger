package signal

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// ErrAlreadyRunning reports a second concurrent Run on the same account.
var ErrAlreadyRunning = errors.New("signal: already running")

// errNotConnected reports an action asked of an account whose Run is
// not currently connected.
var errNotConnected = errors.New("signal: not connected")

// errNotImplemented reports an action this connector cannot perform
// yet: sending and acting on messages are a later addition.
var errNotImplemented = errors.New("signal: not yet implemented")

// opener opens the account's session, pairing it by QR code first if it
// has never linked, and returns the connected-phase client, reporting
// whether this call just completed a fresh link (so Run knows to import
// history). It exists as a field, not a plain call, so tests can run
// the connector's whole lifecycle against a fake client with no
// session database and no network.
type opener func(ctx context.Context, handler func(events.SignalEvent) bool, report func(connector.AuthStep)) (dev device, freshlyLinked bool, err error)

// Connector is one Signal account.
type Connector struct {
	account domain.Account
	open    opener

	mu      sync.Mutex
	running bool
	dev     device
	sink    connector.Sink
	selfACI uuid.UUID

	// polls tracks the options and tallies of every poll this run has
	// seen created, by message remote id, so a later vote or terminate
	// (which arrive with no option text of their own) can be folded in
	// and reported; see poll.go. Bounded by maxTrackedPolls.
	polls      map[string]*pollState
	pollOrder  []string
	identityMu sync.Mutex
	names      map[string]string // resolved contact and group names, by remote id

	// reactions tracks who reacted to each message, by
	// "<conversation remote id>/<message remote id>", so a live
	// reaction event, which carries only the reactor's own emoji, can
	// still report the message's whole set of chips. Bounded by
	// maxTrackedReactions; see reaction.go.
	reactions     map[string]map[uuid.UUID]string
	reactionOrder []string
	reportedChats map[string]bool // conversation remote ids already reported this run

	// attachments remembers each message's own attachment pointer, by
	// "<conversation remote id>/<message remote id>", for FetchMedia
	// (see fetch.go). Bounded by maxTrackedAttachments.
	attachments     map[string]*signalpb.AttachmentPointer
	attachmentOrder []string

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

	return c
}

// Account describes the Signal account.
func (c *Connector) Account() domain.Account {
	return c.account
}

// Run opens the account's session, links it by QR code if it has never
// linked, imports its transferred history on a fresh link, and then
// follows live messages until ctx is cancelled or the connection ends
// for good.
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

	statusCh, err := dev.startReceiveLoops(ctx)
	if err != nil {
		return fmt.Errorf("signal: connect: %w", err)
	}
	defer func() { _ = dev.stopReceiveLoops() }()

	c.connected(dev, sink)
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

// Send is not implemented yet: sending is a later addition.
func (*Connector) Send(context.Context, domain.Conversation, domain.Message) error {
	return errNotImplemented
}

// MarkRead is not implemented yet: sending read receipts is a later
// addition.
func (*Connector) MarkRead(context.Context, domain.Conversation) error {
	return errNotImplemented
}

// Logout tells Signal to unlink this device, if a run is currently
// connected. Manager.Remove calls it, best effort, before stopping
// this connector for good; when nothing is connected there is nothing
// to unlink, so it reports success rather than errNotConnected.
func (c *Connector) Logout(ctx context.Context) error {
	dev, _, err := c.session()
	if err != nil {
		return nil //nolint:nilerr // deliberate: see the doc comment above, nothing connected means nothing to unlink
	}

	return dev.unlink(ctx)
}

// connected records the device and sink of a signed-in run, so Logout,
// LoadOlder, Members and FetchMedia have something to act on.
func (c *Connector) connected(dev device, sink connector.Sink) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev, c.sink, c.selfACI = dev, sink, dev.aci()
}

// selfID returns this run's own account identifier, or uuid.Nil
// before any run has connected.
func (c *Connector) selfID() uuid.UUID {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.selfACI
}

// disconnected forgets the run's device and sink.
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
