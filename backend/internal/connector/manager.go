package connector

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// restartDelays is how long the Manager waits before each restart of a
// failing connector; the last delay repeats.
var restartDelays = [...]time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, time.Minute,
}

// stableRun is how long a connector must run before a failure is treated
// as new rather than part of a run of failures.
const stableRun = 5 * time.Minute

// ErrNoConnector reports an account no connector serves.
var ErrNoConnector = errors.New("no connector for account")

// ErrDuplicateAccount reports a connector for an account already served.
var ErrDuplicateAccount = errors.New("account already has a connector")

// ErrNoMedia reports a media download from a connector that has none.
var ErrNoMedia = errors.New("connector does not download media")

// ErrNoAuthentication reports sign-in input for a connector that does not
// sign in.
var ErrNoAuthentication = errors.New("connector does not sign in")

// ErrNoReactions reports a reaction asked of a connector that does not
// support them.
var ErrNoReactions = errors.New("connector does not support reactions")

// AccountUpserter records the accounts the Manager serves.
type AccountUpserter interface {
	UpsertAccount(ctx context.Context, a domain.Account) error
}

// Manager runs one supervised goroutine per connector, restarting it with
// backoff when it fails, and routes outgoing work to the right connector.
// Connectors can be added and removed while it runs, as accounts are
// added and removed.
type Manager struct {
	initial  []Connector
	requests chan func(ctx context.Context, sink Sink)
	wg       sync.WaitGroup

	mu        sync.Mutex
	byAccount map[string]*running
}

// running is one connector the Manager is supervising.
type running struct {
	conn   Connector
	cancel context.CancelFunc
	done   chan struct{}
}

// NewManager prepares the connectors to start with, rejecting nil
// connectors, empty account ids and duplicates.
func NewManager(connectors ...Connector) (*Manager, error) {
	seen := make(map[string]bool, len(connectors))
	for _, c := range connectors {
		if err := validate(c); err != nil {
			return nil, err
		}

		id := c.Account().ID
		if seen[id] {
			return nil, fmt.Errorf("connector: %w: %q", ErrDuplicateAccount, id)
		}
		seen[id] = true
	}

	return &Manager{
		initial:   connectors,
		requests:  make(chan func(context.Context, Sink)),
		byAccount: make(map[string]*running),
	}, nil
}

// Start records each initial connector's account, then runs connectors
// until ctx is cancelled. Call it once; Wait blocks until every run has
// stopped.
func (m *Manager) Start(ctx context.Context, accounts AccountUpserter, sink Sink) error {
	for _, c := range m.initial {
		if err := accounts.UpsertAccount(ctx, c.Account()); err != nil {
			return fmt.Errorf("connector: record account %q: %w", c.Account().ID, err)
		}
	}

	for _, c := range m.initial {
		m.launch(ctx, c, sink)
	}

	// Connectors added later start inside this loop, which owns ctx.
	m.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case start := <-m.requests:
				start(ctx, sink)
			}
		}
	})

	return nil
}

// Add starts a connector for a newly added account. The caller records
// the account first. It fails if the Manager is not running or the
// account already has a connector.
func (m *Manager) Add(ctx context.Context, c Connector) error {
	if err := validate(c); err != nil {
		return err
	}

	started := make(chan error, 1)
	start := func(runCtx context.Context, sink Sink) {
		if m.has(c.Account().ID) {
			started <- fmt.Errorf("connector: %w: %q", ErrDuplicateAccount, c.Account().ID)
			return
		}

		m.launch(runCtx, c, sink)
		started <- nil
	}

	select {
	case m.requests <- start:
		return <-started
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Remove stops the connector for an account and waits for it to finish,
// so its session files can be deleted safely.
func (m *Manager) Remove(ctx context.Context, accountID string) error {
	m.mu.Lock()
	r, ok := m.byAccount[accountID]
	delete(m.byAccount, accountID)
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("connector: %w %q", ErrNoConnector, accountID)
	}

	r.cancel()
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait blocks until every connector started by Start has stopped.
func (m *Manager) Wait() {
	m.wg.Wait()
}

// Send hands an outgoing message to the connector for its account.
func (m *Manager) Send(ctx context.Context, conv domain.Conversation, msg domain.Message) error {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}

	return c.Send(ctx, conv, msg)
}

// MarkRead hands a read receipt to the connector for its account.
func (m *Manager) MarkRead(ctx context.Context, conv domain.Conversation) error {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}

	return c.MarkRead(ctx, conv)
}

// LoadOlder asks the conversation's connector for older history, if it
// keeps any; one that does not finds nothing.
func (m *Manager) LoadOlder(ctx context.Context, conv domain.Conversation, beforeRemoteID string, limit int) (int, error) {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return 0, err
	}

	loader, ok := c.(HistoryLoader)
	if !ok {
		return 0, nil
	}

	return loader.LoadOlder(ctx, conv, beforeRemoteID, limit)
}

// FetchMedia asks the conversation's connector to download a message's
// media to path.
func (m *Manager) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string) error {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}

	fetcher, ok := c.(MediaFetcher)
	if !ok {
		return fmt.Errorf("connector: %w: %q", ErrNoMedia, conv.AccountID)
	}

	return fetcher.FetchMedia(ctx, conv, messageRemoteID, path)
}

// RefreshMessages asks the conversation's connector to re-report the
// messages named by remoteIDs, if it keeps any; one that does not does
// nothing.
func (m *Manager) RefreshMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string) error {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}

	refresher, ok := c.(MessageRefresher)
	if !ok {
		return nil
	}

	return refresher.RefreshMessages(ctx, conv, remoteIDs)
}

// SetPinned asks the conversation's connector to pin or unpin it with the
// service, if it organizes conversations; one that does not leaves the
// local pin as the only copy.
func (m *Manager) SetPinned(ctx context.Context, conv domain.Conversation, pinned bool) error {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}

	organizer, ok := c.(Organizer)
	if !ok {
		return nil
	}

	return organizer.SetPinned(ctx, conv, pinned)
}

// SetArchived asks the conversation's connector to archive or unarchive it
// with the service, if it organizes conversations; one that does not
// leaves the local archive as the only copy.
func (m *Manager) SetArchived(ctx context.Context, conv domain.Conversation, archived bool) error {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}

	organizer, ok := c.(Organizer)
	if !ok {
		return nil
	}

	return organizer.SetArchived(ctx, conv, archived)
}

// React asks the conversation's connector to set or clear the user's
// reaction to a message, failing with ErrNoReactions when it does not
// support them.
func (m *Manager) React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error {
	c, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}

	reactor, ok := c.(Reactor)
	if !ok {
		return fmt.Errorf("connector: %w: %q", ErrNoReactions, conv.AccountID)
	}

	return reactor.React(ctx, conv, messageRemoteID, emoji)
}

// SubmitAuth hands sign-in input, such as a code, to the connector for
// its account.
func (m *Manager) SubmitAuth(ctx context.Context, accountID, step, value string) error {
	c, err := m.connectorFor(accountID)
	if err != nil {
		return err
	}

	auth, ok := c.(Authenticator)
	if !ok {
		return fmt.Errorf("connector: %w: %q", ErrNoAuthentication, accountID)
	}

	return auth.SubmitAuth(ctx, step, value)
}

// launch supervises c under its own context, so Remove can stop it alone.
func (m *Manager) launch(ctx context.Context, c Connector, sink Sink) {
	runCtx, cancel := context.WithCancel(ctx)
	r := &running{conn: c, cancel: cancel, done: make(chan struct{})}

	m.mu.Lock()
	m.byAccount[c.Account().ID] = r
	m.mu.Unlock()

	m.wg.Go(func() {
		defer close(r.done)
		defer cancel()
		supervise(runCtx, c, sink)
	})
}

// has reports whether an account already has a running connector.
func (m *Manager) has(accountID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.byAccount[accountID]

	return ok
}

// connectorFor returns the connector serving accountID.
func (m *Manager) connectorFor(accountID string) (Connector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.byAccount[accountID]
	if !ok {
		return nil, fmt.Errorf("connector: %w %q", ErrNoConnector, accountID)
	}

	return r.conn, nil
}

// validate rejects a nil connector or one without an account id.
func validate(c Connector) error {
	if c == nil {
		return errors.New("connector: nil connector")
	}

	if c.Account().ID == "" {
		return errors.New("connector: empty account id")
	}

	return nil
}

// supervise runs c until ctx is cancelled. Each failure is reported as an
// account error and followed by a growing delay, which starts again from
// the shortest after a stable run.
func supervise(ctx context.Context, c Connector, sink Sink) {
	accountID := c.Account().ID
	attempt := 0

	for {
		started := time.Now()
		err := c.Run(ctx, sink)
		if ctx.Err() != nil {
			return
		}

		if err == nil {
			err = errors.New("connector stopped unexpectedly")
		}

		sink.AccountStatus(ctx, accountID, domain.AccountError, err.Error())

		if time.Since(started) >= stableRun {
			attempt = 0
		}

		if !sleep(ctx, restartDelays[min(attempt, len(restartDelays)-1)]) {
			return
		}

		attempt++
	}
}

// sleep waits for d, returning false if ctx is cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
