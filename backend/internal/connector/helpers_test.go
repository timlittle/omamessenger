package connector_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// statusSink records account statuses and ignores every other update.
type statusSink struct {
	mu       sync.Mutex
	statuses []string
}

func (s *statusSink) AccountStatus(_ context.Context, accountID, status, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.statuses = append(s.statuses, accountID+":"+status+":"+detail)
}

func (s *statusSink) Contact(context.Context, domain.Contact)                  {}
func (s *statusSink) Conversation(context.Context, domain.Conversation)        {}
func (s *statusSink) Incoming(context.Context, string, string, domain.Message) {}
func (s *statusSink) History(context.Context, string, string, domain.Message)  {}
func (s *statusSink) OutgoingStatus(context.Context, string, string, string)   {}
func (s *statusSink) Typing(context.Context, string, string, string, bool)     {}
func (s *statusSink) Unread(context.Context, string, string, int)              {}
func (s *statusSink) AuthStep(context.Context, string, connector.AuthStep)     {}

// recorded returns a copy of the statuses seen so far.
func (s *statusSink) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.statuses...)
}

// fakeConnector runs the run function, or blocks until cancelled when it
// is nil, and records what it was asked to send.
type fakeConnector struct {
	id   string
	run  func(ctx context.Context) error
	sent []string
}

func (c *fakeConnector) Account() domain.Account {
	return domain.Account{ID: c.id, Service: domain.ServiceWhatsApp, Name: c.id}
}

func (c *fakeConnector) Run(ctx context.Context, _ connector.Sink) error {
	if c.run != nil {
		return c.run(ctx)
	}

	<-ctx.Done()

	return nil
}

func (c *fakeConnector) Send(_ context.Context, _ domain.Conversation, m domain.Message) error {
	c.sent = append(c.sent, m.ID)

	return nil
}

func (c *fakeConnector) MarkRead(context.Context, domain.Conversation) error {
	return nil
}

// signingIn is a connector that signs in, recording what it was given.
type signingIn struct {
	fakeConnector
	answers []string
}

func (c *signingIn) SubmitAuth(_ context.Context, step, value string) error {
	c.answers = append(c.answers, step+"="+value)

	return nil
}

// accountList records upserted account ids, failing when err is set.
type accountList struct {
	ids []string
	err error
}

func (a *accountList) UpsertAccount(_ context.Context, account domain.Account) error {
	a.ids = append(a.ids, account.ID)

	return a.err
}

// errBroken is the failure fake connectors return.
var errBroken = errors.New("broken")

// startManager starts connectors with a statusSink and stops them when the
// test ends.
func startManager(t *testing.T, ctx context.Context, connectors ...connector.Connector) (*connector.Manager, *statusSink) {
	t.Helper()

	m, err := connector.NewManager(connectors...)
	if err != nil {
		t.Fatal(err)
	}

	sink := &statusSink{}
	if err := m.Start(ctx, &accountList{}, sink); err != nil {
		t.Fatal(err)
	}

	return m, sink
}
