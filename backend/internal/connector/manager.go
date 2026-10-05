package connector

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var restartBackoff = [...]time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 60 * time.Second}

// Manager owns connector lifetimes and routes account-scoped operations.
type Manager struct {
	Store      AccountStore
	Sink       Sink
	Clock      Clock
	Connectors []Connector

	mu        sync.RWMutex
	started   bool
	byAccount map[string]Connector
	wg        sync.WaitGroup
}

// Start persists connector accounts and starts one supervised goroutine per
// connector. It may be called once; Wait blocks until all runs have stopped.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.validate(ctx); err != nil {
		return err
	}
	byAccount, accounts, err := indexByAccount(m.Connectors)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if err := m.Store.UpsertAccount(account); err != nil {
			return fmt.Errorf("upsert account %q: %w", account.ID, err)
		}
	}
	if err := m.claimStart(byAccount); err != nil {
		return err
	}
	for _, connector := range m.Connectors {
		go m.run(ctx, connector)
	}
	return nil
}

// validate checks the Manager's dependencies and defaults its Clock.
func (m *Manager) validate(ctx context.Context) error {
	switch {
	case ctx == nil:
		return errors.New("connector manager requires a context")
	case m.Store == nil:
		return errors.New("connector manager requires a store")
	case m.Sink == nil:
		return errors.New("connector manager requires a sink")
	case m.isStarted():
		return errors.New("connector manager already started")
	}
	if m.Clock == nil {
		m.Clock = RealClock{}
	}
	return nil
}

func (m *Manager) isStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// indexByAccount maps each connector to its account id, rejecting nil
// connectors, empty ids and duplicates.
func indexByAccount(connectors []Connector) (map[string]Connector, []domain.Account, error) {
	byAccount := make(map[string]Connector, len(connectors))
	accounts := make([]domain.Account, 0, len(connectors))
	for _, connector := range connectors {
		if connector == nil {
			return nil, nil, errors.New("connector manager received a nil connector")
		}
		account := connector.Account()
		if account.ID == "" {
			return nil, nil, errors.New("connector account id is empty")
		}
		if _, exists := byAccount[account.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate connector account %q", account.ID)
		}
		byAccount[account.ID] = connector
		accounts = append(accounts, account)
	}
	return byAccount, accounts, nil
}

// claimStart records the routing table and marks the Manager started, unless
// a concurrent Start got there first.
func (m *Manager) claimStart(byAccount map[string]Connector) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return errors.New("connector manager already started")
	}
	m.started = true
	m.byAccount = byAccount
	m.wg.Add(len(m.Connectors))
	return nil
}

// Wait blocks until every connector goroutine started by Start has exited.
func (m *Manager) Wait() { m.wg.Wait() }

// Send routes a normalized outgoing message to the connector for conv's
// account. Connectors return quickly and report delivery progress through Sink.
func (m *Manager) Send(ctx context.Context, conv domain.Conversation, message domain.Message) error {
	connector, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}
	return connector.Send(ctx, conv, message)
}

// MarkRead routes a read receipt to the connector for conv's account.
func (m *Manager) MarkRead(ctx context.Context, conv domain.Conversation) error {
	connector, err := m.connectorFor(conv.AccountID)
	if err != nil {
		return err
	}
	return connector.MarkRead(ctx, conv)
}

func (m *Manager) connectorFor(accountID string) (Connector, error) {
	m.mu.RLock()
	connector := m.byAccount[accountID]
	m.mu.RUnlock()
	if connector == nil {
		return nil, fmt.Errorf("no connector for account %q", accountID)
	}
	return connector, nil
}

func (m *Manager) run(ctx context.Context, connector Connector) {
	defer m.wg.Done()
	accountID := connector.Account().ID
	backoffIndex := 0
	for ctx.Err() == nil {
		state := &connectionState{}
		err := connector.Run(ctx, trackedSink{Sink: m.Sink, Clock: m.Clock, AccountID: accountID, State: state})
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("connector stopped unexpectedly")
		}
		m.Sink.AccountStatus(accountID, domain.AccountError, err.Error())

		delay := restartBackoff[backoffIndex]
		if state.connectedDuration(m.Clock.Now()) >= 5*time.Minute {
			backoffIndex = 0
			delay = restartBackoff[backoffIndex]
		}
		if !wait(ctx, m.Clock, delay) {
			return
		}
		if backoffIndex < len(restartBackoff)-1 {
			backoffIndex++
		}
	}
}

func wait(ctx context.Context, clock Clock, duration time.Duration) bool {
	done := make(chan struct{})
	stop := clock.AfterFunc(duration, func() { close(done) })
	select {
	case <-ctx.Done():
		stop()
		return false
	case <-done:
		return ctx.Err() == nil
	}
}

type connectionState struct {
	mu           sync.Mutex
	connectedAt  *time.Time
	connectedFor time.Duration
}

func (s *connectionState) connectedDuration(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	duration := s.connectedFor
	if s.connectedAt != nil {
		duration += now.Sub(*s.connectedAt)
	}
	return duration
}

type trackedSink struct {
	Sink
	Clock     Clock
	AccountID string
	State     *connectionState
}

func (s trackedSink) AccountStatus(accountID, status, detail string) {
	if accountID == s.AccountID {
		s.State.mu.Lock()
		if status == domain.AccountConnected {
			if s.State.connectedAt == nil {
				now := s.Clock.Now()
				s.State.connectedAt = &now
			}
		} else if s.State.connectedAt != nil {
			s.State.connectedFor += s.Clock.Now().Sub(*s.State.connectedAt)
			s.State.connectedAt = nil
		}
		s.State.mu.Unlock()
	}
	s.Sink.AccountStatus(accountID, status, detail)
}

func (s trackedSink) History(accountID, conversationRemoteID string, message domain.Message) {
	if history, ok := s.Sink.(HistorySink); ok {
		history.History(accountID, conversationRemoteID, message)
		return
	}
	s.Sink.Incoming(accountID, conversationRemoteID, message)
}
