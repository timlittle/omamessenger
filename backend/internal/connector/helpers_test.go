package connector_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

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

// withHistory is a connector that loads older history, recording where
// from, and says it found count messages.
type withHistory struct {
	fakeConnector
	from  []string
	count int
}

func (c *withHistory) LoadOlder(_ context.Context, _ domain.Conversation, beforeRemoteID string, _ int) (int, error) {
	c.from = append(c.from, beforeRemoteID)

	return c.count, nil
}

// withMedia is a connector that downloads media, recording which message
// and where to.
type withMedia struct {
	fakeConnector
	fetched []string
}

func (c *withMedia) FetchMedia(_ context.Context, _ domain.Conversation, messageRemoteID, path string) error {
	c.fetched = append(c.fetched, messageRemoteID+" "+path)

	return nil
}

// withRefresh is a connector that re-reports messages, recording which
// ones it was asked for.
type withRefresh struct {
	fakeConnector
	asked [][]string
}

func (c *withRefresh) RefreshMessages(_ context.Context, _ domain.Conversation, remoteIDs []string) error {
	c.asked = append(c.asked, remoteIDs)

	return nil
}

// withOrganizer is a connector that pins and archives, recording what it
// was asked.
type withOrganizer struct {
	fakeConnector
	pinned   []bool
	archived []bool
}

func (c *withOrganizer) SetPinned(_ context.Context, _ domain.Conversation, pinned bool) error {
	c.pinned = append(c.pinned, pinned)

	return nil
}

func (c *withOrganizer) SetArchived(_ context.Context, _ domain.Conversation, archived bool) error {
	c.archived = append(c.archived, archived)

	return nil
}

// withReactor is a connector that reacts to messages, recording what it
// was asked to set.
type withReactor struct {
	fakeConnector
	reacted []string
}

func (c *withReactor) React(_ context.Context, _ domain.Conversation, messageRemoteID, emoji string) error {
	c.reacted = append(c.reacted, messageRemoteID+" "+emoji)

	return nil
}

// withDeleter is a connector that deletes messages, recording what it
// was asked to delete.
type withDeleter struct {
	fakeConnector
	deleted []string
}

func (c *withDeleter) DeleteMessages(_ context.Context, _ domain.Conversation, ids []string, forEveryone bool) error {
	c.deleted = append(c.deleted, fmt.Sprintf("%v %t", ids, forEveryone))

	return nil
}

// withLogout is a connector that can unlink itself from its service,
// recording whether it was asked to and reporting logoutErr.
type withLogout struct {
	fakeConnector
	loggedOut bool
	logoutErr error
}

func (c *withLogout) Logout(context.Context) error {
	c.loggedOut = true

	return c.logoutErr
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

// startManager starts connectors with a recording sink and stops them when the
// test ends.
func startManager(t *testing.T, ctx context.Context, connectors ...connector.Connector) (*connector.Manager, *connectortest.Sink) {
	t.Helper()

	m, err := connector.NewManager(connectors...)
	if err != nil {
		t.Fatal(err)
	}

	sink := &connectortest.Sink{}
	if err := m.Start(ctx, &accountList{}, sink); err != nil {
		t.Fatal(err)
	}

	return m, sink
}
