package connector_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

type sinkEvent struct {
	accountID string
	status    string
	detail    string
}

type recordingSink struct{ statuses chan sinkEvent }

func newRecordingSink() *recordingSink {
	return &recordingSink{statuses: make(chan sinkEvent, 100)}
}

func (s *recordingSink) AccountStatus(id, status, detail string) {
	s.statuses <- sinkEvent{accountID: id, status: status, detail: detail}
}
func (*recordingSink) Contact(domain.Contact)                  {}
func (*recordingSink) Conversation(domain.Conversation)        {}
func (*recordingSink) Incoming(string, string, domain.Message) {}
func (*recordingSink) OutgoingStatus(string, string, string)   {}
func (*recordingSink) Typing(string, string, string, bool)     {}

type fakeConnector struct {
	account       domain.Account
	run           func(context.Context, connector.Sink) error
	send          func(context.Context, domain.Conversation, domain.Message) error
	markRead      func(context.Context, domain.Conversation) error
	defaultRunErr error
}

func (c *fakeConnector) Account() domain.Account { return c.account }
func (c *fakeConnector) Run(ctx context.Context, sink connector.Sink) error {
	if c.run != nil {
		return c.run(ctx, sink)
	}
	if c.defaultRunErr != nil {
		return c.defaultRunErr
	}
	<-ctx.Done()
	return nil
}
func (c *fakeConnector) Send(ctx context.Context, conv domain.Conversation, msg domain.Message) error {
	if c.send != nil {
		return c.send(ctx, conv, msg)
	}
	return nil
}
func (c *fakeConnector) MarkRead(ctx context.Context, conv domain.Conversation) error {
	if c.markRead != nil {
		return c.markRead(ctx, conv)
	}
	return nil
}

func newManager(t *testing.T, fakeClock connector.Clock, sink connector.Sink, connectors ...connector.Connector) *connector.Manager {
	t.Helper()
	return &connector.Manager{Store: newMemoryAccounts(), Sink: sink, Clock: fakeClock, Connectors: connectors}
}

func testAccount(id string) domain.Account {
	return domain.Account{ID: id, Service: domain.ServiceWhatsApp, Name: id}
}

func waitFor(t *testing.T, description string, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func receiveStatus(t *testing.T, statuses <-chan sinkEvent, want string) sinkEvent {
	t.Helper()
	select {
	case event := <-statuses:
		if event.status != want {
			t.Fatalf("account status = %#v, want %q", event, want)
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for account status %q", want)
		return sinkEvent{}
	}
}

func TestManagerPersistsAccountsAndValidatesStart(t *testing.T) {
	sink := newRecordingSink()
	clock := clocktest.New(time.Unix(1, 0))
	manager := newManager(t, clock, sink,
		&fakeConnector{account: testAccount("wa")},
		&fakeConnector{account: domain.Account{ID: "tg", Service: domain.ServiceTelegram, Name: "Work"}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"wa", "tg"} {
		account, ok := manager.Store.(*memoryAccounts).get(id)
		if !ok || account.ID != id {
			t.Errorf("Start() did not persist account %q: %#v", id, account)
		}
	}
	if err := manager.Start(ctx); err == nil {
		t.Error("second Start() succeeded")
	}
	cancel()
	manager.Wait()
}

func TestManagerStartRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name string
		make func(*testing.T) (*connector.Manager, context.Context)
	}{
		{
			name: "nil context",
			make: func(t *testing.T) (*connector.Manager, context.Context) {
				manager := newManager(t, nil, newRecordingSink())
				return manager, nil
			},
		},
		{
			name: "nil store",
			make: func(*testing.T) (*connector.Manager, context.Context) {
				return &connector.Manager{Sink: newRecordingSink()}, context.Background()
			},
		},
		{
			name: "nil sink",
			make: func(t *testing.T) (*connector.Manager, context.Context) {
				manager := newManager(t, nil, nil)
				return manager, context.Background()
			},
		},
		{
			name: "nil connector",
			make: func(t *testing.T) (*connector.Manager, context.Context) {
				manager := newManager(t, nil, newRecordingSink(), nil)
				return manager, context.Background()
			},
		},
		{
			name: "empty account id",
			make: func(t *testing.T) (*connector.Manager, context.Context) {
				manager := newManager(t, nil, newRecordingSink(), &fakeConnector{account: domain.Account{Service: domain.ServiceWhatsApp, Name: "No ID"}})
				return manager, context.Background()
			},
		},
		{
			name: "duplicate account id",
			make: func(t *testing.T) (*connector.Manager, context.Context) {
				manager := newManager(t, nil, newRecordingSink(), &fakeConnector{account: testAccount("dup")}, &fakeConnector{account: testAccount("dup")})
				return manager, context.Background()
			},
		},
		{
			name: "invalid account rejected by store",
			make: func(t *testing.T) (*connector.Manager, context.Context) {
				manager := newManager(t, nil, newRecordingSink(), &fakeConnector{account: domain.Account{ID: "bad", Service: "invalid", Name: "Bad"}})
				return manager, context.Background()
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, ctx := tt.make(t)
			if err := manager.Start(ctx); err == nil {
				t.Fatal("Start() accepted invalid configuration")
			}
		})
	}
}

func TestManagerRestartBackoffAndCancellation(t *testing.T) {
	clock := clocktest.New(time.Unix(100, 0))
	sink := newRecordingSink()
	starts := make(chan int, 10)
	var runMu sync.Mutex
	runCount := 0
	conn := &fakeConnector{account: testAccount("wa")}
	conn.run = func(context.Context, connector.Sink) error {
		runMu.Lock()
		runCount++
		count := runCount
		runMu.Unlock()
		starts <- count
		return fmt.Errorf("failure-%d", count)
	}
	manager := newManager(t, clock, sink, conn)
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case first := <-starts:
		if first != 1 {
			t.Fatalf("first run number = %d", first)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first connector run did not start")
	}
	firstError := receiveStatus(t, sink.statuses, domain.AccountError)
	if firstError.detail != "failure-1" {
		t.Errorf("error detail = %q, want failure-1", firstError.detail)
	}

	backoffs := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 60 * time.Second, 60 * time.Second}
	for i, delay := range backoffs {
		waitFor(t, "restart timer registration", func() bool { return clock.Pending() > 0 })
		clock.Advance(delay)
		select {
		case count := <-starts:
			if count != i+2 {
				t.Fatalf("run after backoff %d = %d, want %d", i, count, i+2)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("connector did not restart after %s", delay)
		}
		if i != len(backoffs)-1 {
			if event := receiveStatus(t, sink.statuses, domain.AccountError); event.detail != fmt.Sprintf("failure-%d", i+2) {
				t.Errorf("restart error detail = %q", event.detail)
			}
		}
	}
	waitFor(t, "last restart timer registration", func() bool { return clock.Pending() > 0 })
	cancel()
	done := make(chan struct{})
	go func() { manager.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("connector goroutine did not stop after context cancellation")
	}
	waitFor(t, "canceled timer removal", func() bool { return clock.Pending() == 0 })
}

func TestManagerResetsBackoffAfterStableConnection(t *testing.T) {
	clock := clocktest.New(time.Unix(200, 0))
	sink := newRecordingSink()
	firstConnected := make(chan struct{})
	stopFirst := make(chan struct{})
	restarted := make(chan struct{})
	var runMu sync.Mutex
	runCount := 0
	conn := &fakeConnector{account: testAccount("wa")}
	conn.run = func(ctx context.Context, sink connector.Sink) error {
		runMu.Lock()
		runCount++
		count := runCount
		runMu.Unlock()
		if count == 1 {
			sink.AccountStatus("wa", domain.AccountConnected, "")
			close(firstConnected)
			select {
			case <-stopFirst:
				return errors.New("connection lost")
			case <-ctx.Done():
				return nil
			}
		}
		close(restarted)
		<-ctx.Done()
		return nil
	}
	manager := newManager(t, clock, sink, conn)
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("connector did not report connected")
	}
	clock.Advance(5 * time.Minute)
	close(stopFirst)
	if event := receiveStatus(t, sink.statuses, domain.AccountConnected); event.accountID != "wa" {
		t.Fatalf("connected event = %#v", event)
	}
	if event := receiveStatus(t, sink.statuses, domain.AccountError); event.detail != "connection lost" {
		t.Fatalf("failure event = %#v", event)
	}
	waitFor(t, "retry timer registration", func() bool { return clock.Pending() > 0 })
	clock.Advance(time.Second)
	select {
	case <-restarted:
	case <-time.After(2 * time.Second):
		t.Fatal("stable connection did not reset retry delay to one second")
	}
	cancel()
	manager.Wait()
}

func TestManagerRoutesSendAndMarkRead(t *testing.T) {
	sink := newRecordingSink()
	var sent domain.Message
	var sentConversation domain.Conversation
	var readConversation domain.Conversation
	conn := &fakeConnector{
		account: testAccount("wa"),
		send: func(_ context.Context, conv domain.Conversation, msg domain.Message) error {
			sentConversation, sent = conv, msg
			return nil
		},
		markRead: func(_ context.Context, conv domain.Conversation) error {
			readConversation = conv
			return nil
		},
	}
	manager := newManager(t, nil, sink, conn)
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); manager.Wait() }()
	conv := domain.Conversation{ID: "chat", AccountID: "wa"}
	msg := domain.Message{ID: "message", Text: "hi"}
	if err := manager.Send(ctx, conv, msg); err != nil || sent.ID != msg.ID || sentConversation.ID != conv.ID {
		t.Fatalf("Send() routed %#v/%#v: %v", sentConversation, sent, err)
	}
	if err := manager.MarkRead(ctx, conv); err != nil || readConversation.ID != conv.ID {
		t.Fatalf("MarkRead() routed %#v: %v", readConversation, err)
	}
	if err := manager.Send(ctx, domain.Conversation{AccountID: "missing"}, msg); err == nil {
		t.Error("Send() accepted an unknown account")
	}
	if err := manager.MarkRead(ctx, domain.Conversation{AccountID: "missing"}); err == nil {
		t.Error("MarkRead() accepted an unknown account")
	}
}

func TestFakeClockRunsCallbacksByTimeAndRegistrationOrder(t *testing.T) {
	clock := clocktest.New(time.Unix(300, 0))
	var mu sync.Mutex
	var fired []string
	appendFired := func(name string) {
		mu.Lock()
		fired = append(fired, name)
		mu.Unlock()
	}
	clock.AfterFunc(2*time.Second, func() { appendFired("late") })
	clock.AfterFunc(time.Second, func() { appendFired("first") })
	clock.AfterFunc(time.Second, func() { appendFired("second") })
	stopped := clock.AfterFunc(time.Second, func() { appendFired("stopped") })
	if !stopped() || stopped() {
		t.Fatal("timer stop should succeed exactly once")
	}
	clock.Advance(2 * time.Second)
	if want := []string{"first", "second", "late"}; fmt.Sprint(fired) != fmt.Sprint(want) {
		t.Errorf("callback order = %v, want %v", fired, want)
	}
	if got := clock.Now(); !got.Equal(time.Unix(300, 0).Add(2 * time.Second)) {
		t.Errorf("clock now = %v", got)
	}
}

// memoryAccounts is an in-memory connector.AccountStore. Like the real store
// it rejects accounts with an unknown service.
type memoryAccounts struct {
	mu       sync.Mutex
	accounts map[string]domain.Account
}

func newMemoryAccounts() *memoryAccounts {
	return &memoryAccounts{accounts: map[string]domain.Account{}}
}

func (m *memoryAccounts) UpsertAccount(account domain.Account) error {
	if !domain.ValidService(account.Service) {
		return fmt.Errorf("invalid account %q", account.ID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts[account.ID] = account
	return nil
}

func (m *memoryAccounts) get(id string) (domain.Account, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	account, ok := m.accounts[id]
	return account, ok
}
