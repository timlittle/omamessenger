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

// AccountUpserter records the accounts the Manager serves.
type AccountUpserter interface {
	UpsertAccount(ctx context.Context, a domain.Account) error
}

// Manager runs one supervised goroutine per connector, restarting it with
// backoff when it fails, and routes outgoing work to the right connector.
type Manager struct {
	connectors []Connector
	byAccount  map[string]Connector
	wg         sync.WaitGroup
}

// NewManager indexes connectors by account, rejecting nil connectors,
// empty account ids and duplicates.
func NewManager(connectors ...Connector) (*Manager, error) {
	m := &Manager{connectors: connectors, byAccount: make(map[string]Connector, len(connectors))}

	for _, c := range connectors {
		if c == nil {
			return nil, errors.New("connector: nil connector")
		}

		id := c.Account().ID
		if id == "" {
			return nil, errors.New("connector: empty account id")
		}

		if _, dup := m.byAccount[id]; dup {
			return nil, fmt.Errorf("connector: duplicate account %q", id)
		}

		m.byAccount[id] = c
	}

	return m, nil
}

// Start records each connector's account, then runs every connector until
// ctx is cancelled. Call it once; Wait blocks until the runs have stopped.
func (m *Manager) Start(ctx context.Context, accounts AccountUpserter, sink Sink) error {
	for _, c := range m.connectors {
		if err := accounts.UpsertAccount(ctx, c.Account()); err != nil {
			return fmt.Errorf("connector: record account %q: %w", c.Account().ID, err)
		}
	}

	for _, c := range m.connectors {
		m.wg.Go(func() { supervise(ctx, c, sink) })
	}

	return nil
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

// connectorFor returns the connector serving accountID.
func (m *Manager) connectorFor(accountID string) (Connector, error) {
	c, ok := m.byAccount[accountID]
	if !ok {
		return nil, fmt.Errorf("connector: %w %q", ErrNoConnector, accountID)
	}

	return c, nil
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
