package app_test

// connector_fakes_test.go holds the fakes for the connector-facing
// capabilities Commands depends on (Deps.Refresher, Organizer, Reactor,
// Voter, Deleter, Editor, Members): split out of helpers_test.go, which
// was already near the file-length gate, the same reasoning that split
// outgoing.go from ingest.go (see docs/decisions.md).

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fakeRefresher plays a service's response to being asked to re-report
// messages: for each remote id it is given a message to report, it reports
// that message through the ingest, as a connector does through its Sink.
// It records every batch it was asked to refresh.
type fakeRefresher struct {
	ingest *app.Ingest
	toSend map[string]domain.Message
	asked  [][]string
	err    error
}

func (r *fakeRefresher) RefreshMessages(ctx context.Context, conv domain.Conversation, remoteIDs []string) error {
	r.asked = append(r.asked, remoteIDs)
	if r.err != nil {
		return r.err
	}

	for _, id := range remoteIDs {
		if m, ok := r.toSend[id]; ok {
			r.ingest.History(ctx, conv.AccountID, conv.RemoteID, m)
		}
	}

	return nil
}

// fakeOrganizer records the pins and archives it is asked for, as
// "<conversationId> <value>", failing with err when it is set.
type fakeOrganizer struct {
	mu       sync.Mutex
	pinned   []string
	archived []string
	err      error
}

func (o *fakeOrganizer) SetPinned(_ context.Context, conv domain.Conversation, pinned bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.pinned = append(o.pinned, fmt.Sprintf("%s %t", conv.ID, pinned))

	return o.err
}

func (o *fakeOrganizer) SetArchived(_ context.Context, conv domain.Conversation, archived bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.archived = append(o.archived, fmt.Sprintf("%s %t", conv.ID, archived))

	return o.err
}

// fakeMemberLister reports a scripted member list, failing with err
// when it is set.
type fakeMemberLister struct {
	mu      sync.Mutex
	members []domain.Member
	err     error
	asked   []string
}

func (l *fakeMemberLister) Members(_ context.Context, conv domain.Conversation) ([]domain.Member, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.asked = append(l.asked, conv.ID)

	return l.members, l.err
}

// fakeReactor records the reactions it is asked for, as
// "<messageRemoteId> <emoji>", failing with err when it is set.
type fakeReactor struct {
	mu      sync.Mutex
	reacted []string
	err     error
}

func (r *fakeReactor) React(_ context.Context, _ domain.Conversation, messageRemoteID, emoji string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reacted = append(r.reacted, messageRemoteID+" "+emoji)

	return r.err
}

// fakeVoter records the votes it is asked to cast, as "messageRemoteID
// optionIDs", failing with err when set.
type fakeVoter struct {
	mu    sync.Mutex
	voted []string
	err   error
}

func (v *fakeVoter) Vote(_ context.Context, _ domain.Conversation, messageRemoteID string, optionIDs []string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.voted = append(v.voted, fmt.Sprintf("%s %v", messageRemoteID, optionIDs))

	return v.err
}

// fakeDeleter records the deletes it is asked for, as
// "<conversationId> <remoteIds> <forEveryone>", failing with err when
// it is set.
type fakeDeleter struct {
	mu      sync.Mutex
	deleted []string
	err     error
}

func (d *fakeDeleter) DeleteMessages(_ context.Context, conv domain.Conversation, remoteIDs []string, forEveryone bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.deleted = append(d.deleted, fmt.Sprintf("%s %s %t", conv.ID, strings.Join(remoteIDs, ","), forEveryone))

	return d.err
}

// fakeEditor records the edits it is asked for, as "<messageRemoteId>
// <newText>", failing with err when it is set.
type fakeEditor struct {
	mu     sync.Mutex
	edited []string
	err    error

	// onEditMessage, when set, runs after recording the call and
	// before EditMessage returns, for the same reason
	// fakeBlocker.onSetBlocked does.
	onEditMessage func()
}

func (e *fakeEditor) EditMessage(_ context.Context, _ domain.Conversation, messageRemoteID, newText string, _ []domain.Mention) error {
	e.mu.Lock()
	e.edited = append(e.edited, messageRemoteID+" "+newText)
	hook, err := e.onEditMessage, e.err
	e.mu.Unlock()

	if hook != nil {
		hook()
	}

	return err
}

// fakeBlocker records the blocks and unblocks it is asked for, as
// "<conversationId> <blocked>", failing with err when it is set.
type fakeBlocker struct {
	mu      sync.Mutex
	blocked []string
	err     error

	// onSetBlocked, when set, runs after recording the call and before
	// SetBlocked returns, so a test can change store state out from
	// under the caller - such as deleting the conversation - between
	// the service call succeeding and the caller's own store write.
	onSetBlocked func()
}

func (b *fakeBlocker) SetBlocked(_ context.Context, conv domain.Conversation, blocked bool) error {
	b.mu.Lock()
	b.blocked = append(b.blocked, fmt.Sprintf("%s %t", conv.ID, blocked))
	hook, err := b.onSetBlocked, b.err
	b.mu.Unlock()

	if hook != nil {
		hook()
	}

	return err
}

// fakeSpamReporter counts the spam reports it is asked for, failing
// with err when it is set.
type fakeSpamReporter struct {
	mu       sync.Mutex
	reported int
	err      error

	// onReportSpam, when set, runs after recording the call and before
	// ReportSpam returns, for the same reason fakeBlocker.onSetBlocked
	// does.
	onReportSpam func()
}

func (r *fakeSpamReporter) ReportSpam(_ context.Context, _ domain.Conversation) error {
	r.mu.Lock()
	r.reported++
	hook, err := r.onReportSpam, r.err
	r.mu.Unlock()

	if hook != nil {
		hook()
	}

	return err
}
