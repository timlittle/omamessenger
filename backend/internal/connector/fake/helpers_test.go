package fake_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// recordingSink records every update as a short line, such as
// "status wa-personal connected" or "incoming wa:mum fake-reply-m1".
type recordingSink struct {
	mu      sync.Mutex
	events  []string
	history map[string][]domain.Message
}

func newRecordingSink() *recordingSink {
	return &recordingSink{history: make(map[string][]domain.Message)}
}

func (s *recordingSink) record(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, fmt.Sprintf(format, args...))
}

func (s *recordingSink) AccountStatus(_ context.Context, accountID, status, _ string) {
	s.record("status %s %s", accountID, status)
}

func (s *recordingSink) Contact(_ context.Context, c domain.Contact) {
	s.record("contact %s %s", c.AccountID, c.Name)
}

func (s *recordingSink) Conversation(_ context.Context, c domain.Conversation) {
	s.record("conversation %s", c.RemoteID)
}

func (s *recordingSink) Incoming(_ context.Context, _, remoteID string, m domain.Message) {
	s.record("incoming %s %s", remoteID, m.RemoteID)
}

func (s *recordingSink) History(_ context.Context, _, remoteID string, m domain.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history[remoteID] = append(s.history[remoteID], m)
}

func (s *recordingSink) OutgoingStatus(_ context.Context, localID, remoteID, status string) {
	s.record("outgoing %s %s %s", localID, remoteID, status)
}

func (s *recordingSink) AuthStep(_ context.Context, accountID string, step connector.AuthStep) {
	s.record("auth %s %s", accountID, step.Kind)
}

func (s *recordingSink) Typing(_ context.Context, _, remoteID, _ string, active bool) {
	s.record("typing %s %t", remoteID, active)
}

// take returns the events recorded so far and forgets them.
func (s *recordingSink) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := s.events
	s.events = nil

	return events
}

// has reports whether event was recorded since the last take.
func (s *recordingSink) has(event string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Contains(s.events, event)
}

// runFake starts every fake connector inside the current synctest bubble
// and returns a function that stops them and waits for Run to return.
func runFake(t *testing.T, suite *fake.Suite, sink connector.Sink) (stop func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	for _, c := range suite.Connectors() {
		wg.Go(func() {
			if err := c.Run(ctx, sink); err != nil {
				t.Errorf("Run(%s) = %v", c.Account().ID, err)
			}
		})
	}

	return func() {
		cancel()
		wg.Wait()
	}
}

// conversation returns a fake conversation as the app would pass it.
func conversation(accountID, remoteID, kind string) domain.Conversation {
	return domain.Conversation{AccountID: accountID, RemoteID: remoteID, Kind: kind}
}

// waitConnected lets every fake account connect, then forgets the events
// so far. It must run inside a synctest bubble.
func waitConnected(sink *recordingSink) {
	time.Sleep(2 * time.Second)
	synctest.Wait()
	sink.take()
}
