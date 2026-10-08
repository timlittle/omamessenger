// Package connectortest records what connectors report, for the tests of
// the connector packages.
package connectortest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Sink records every update as a short line, such as
// "status tg connected" or "incoming user:42:99 7", and keeps earlier
// messages by conversation.
type Sink struct {
	mu            sync.Mutex
	lines         []string
	history       map[string][]domain.Message
	live          map[string][]domain.Message
	outgoing      map[string][]OutgoingUpdate
	authSteps     []connector.AuthStep
	conversations map[string]domain.Conversation
	unread        map[string]int
	organized     map[string]organizedFlags
	deleted       map[string]map[string]bool
}

// organizedFlags is a conversation's last reported pinned and archived
// state, for Snapshot.
type organizedFlags struct {
	pinned, archived bool
}

// OutgoingUpdate is one delivery status reported for a message the local
// side sent, with the service's id for it once known.
type OutgoingUpdate struct {
	RemoteID string
	Status   string
}

var _ connector.Sink = (*Sink)(nil)

// record adds one line, without trailing spaces from empty fields.
func (s *Sink) record(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lines = append(s.lines, strings.TrimRight(fmt.Sprintf(format, args...), " "))
}

// Lines returns the lines recorded so far.
func (s *Sink) Lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.lines)
}

// Take returns the lines recorded so far and forgets them.
func (s *Sink) Take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	lines := s.lines
	s.lines = nil

	return lines
}

// Has reports whether line was recorded since the last Take.
func (s *Sink) Has(line string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Contains(s.lines, line)
}

// Messages returns the earlier messages reported, by conversation.
func (s *Sink) Messages() map[string][]domain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string][]domain.Message, len(s.history))
	for remote, messages := range s.history {
		out[remote] = slices.Clone(messages)
	}

	return out
}

// AccountStatus records a status change and its detail.
func (s *Sink) AccountStatus(_ context.Context, accountID, status, detail string) {
	s.record("status %s %s %s", accountID, status, detail)
}

// Contact records a contact.
func (s *Sink) Contact(_ context.Context, c domain.Contact) {
	s.record("contact %s %s", c.RemoteID, c.Name)
}

// Conversation records a conversation and keeps its full data, so a
// test can inspect fields the short line leaves out, such as a group's
// member count.
func (s *Sink) Conversation(_ context.Context, c domain.Conversation) {
	s.record("conversation %s %s", c.RemoteID, c.Title)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conversations == nil {
		s.conversations = map[string]domain.Conversation{}
	}
	s.conversations[c.RemoteID] = c
}

// ConversationFor returns the last conversation reported for remoteID,
// and whether one was reported at all.
func (s *Sink) ConversationFor(remoteID string) (domain.Conversation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.conversations[remoteID]

	return c, ok
}

// Incoming records a new message and keeps it, like History does for
// earlier ones, so a test can inspect what a connector reported live.
func (s *Sink) Incoming(_ context.Context, _, remote string, m domain.Message) {
	s.record("incoming %s %s", remote, m.RemoteID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.live == nil {
		s.live = map[string][]domain.Message{}
	}
	s.live[remote] = append(s.live[remote], m)
}

// LiveMessages returns the live messages reported, by conversation.
func (s *Sink) LiveMessages() map[string][]domain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string][]domain.Message, len(s.live))
	for remote, messages := range s.live {
		out[remote] = slices.Clone(messages)
	}

	return out
}

// History records an earlier message and keeps it.
func (s *Sink) History(_ context.Context, _, remote string, m domain.Message) {
	s.record("history %s %s", remote, m.RemoteID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.history == nil {
		s.history = map[string][]domain.Message{}
	}
	s.history[remote] = append(s.history[remote], m)
}

// Edited records a message changed after it was sent.
func (s *Sink) Edited(_ context.Context, _, remote string, m domain.Message) {
	s.record("edited %s %s", remote, m.RemoteID)
}

// Reacted records a message's reaction chips changing on their own.
func (s *Sink) Reacted(_ context.Context, _, remote, messageRemoteID string, reactions []domain.Reaction) {
	s.record("reacted %s %s %d", remote, messageRemoteID, len(reactions))
}

// Deleted records messages removed from the service, and marks each of
// remoteIDs removed from every conversation named in
// conversationRemoteIDs, for Snapshot.
func (s *Sink) Deleted(_ context.Context, _ string, conversationRemoteIDs, remoteIDs []string) {
	s.record("deleted %s %s", strings.Join(conversationRemoteIDs, ","), strings.Join(remoteIDs, ","))

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.deleted == nil {
		s.deleted = map[string]map[string]bool{}
	}
	for _, conv := range conversationRemoteIDs {
		if s.deleted[conv] == nil {
			s.deleted[conv] = map[string]bool{}
		}
		for _, id := range remoteIDs {
			s.deleted[conv][id] = true
		}
	}
}

// OutgoingStatus records a change to a sent message and keeps the update
// so a test can check the sequence of statuses for that message.
func (s *Sink) OutgoingStatus(_ context.Context, localID, remoteID, status string) {
	s.record("outgoing %s %s %s", localID, remoteID, status)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.outgoing == nil {
		s.outgoing = map[string][]OutgoingUpdate{}
	}
	s.outgoing[localID] = append(s.outgoing[localID], OutgoingUpdate{RemoteID: remoteID, Status: status})
}

// Outgoing returns the delivery updates recorded for a message we sent, in
// the order they arrived.
func (s *Sink) Outgoing(localMessageID string) []OutgoingUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.outgoing[localMessageID])
}

// Typing records someone starting or stopping typing.
func (s *Sink) Typing(_ context.Context, _, remote, _ string, active bool) {
	s.record("typing %s %t", remote, active)
}

// Unread records the service's unread count for a conversation, and
// keeps it for Snapshot.
func (s *Sink) Unread(_ context.Context, _, remote string, count int) {
	s.record("unread %s %d", remote, count)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.unread == nil {
		s.unread = map[string]int{}
	}
	s.unread[remote] = count
}

// Organized records a conversation's pinned and archived state, and
// keeps it for Snapshot.
func (s *Sink) Organized(_ context.Context, _, remote string, pinned, archived bool) {
	s.record("organized %s %t %t", remote, pinned, archived)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.organized == nil {
		s.organized = map[string]organizedFlags{}
	}
	s.organized[remote] = organizedFlags{pinned: pinned, archived: archived}
}

// AuthStep records a sign-in step and keeps it, so a test can inspect
// its QR image or hint, not just its kind.
func (s *Sink) AuthStep(_ context.Context, accountID string, step connector.AuthStep) {
	s.record("auth %s %s", accountID, step.Kind)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.authSteps = append(s.authSteps, step)
}

// AuthSteps returns the sign-in steps reported so far, in order.
func (s *Sink) AuthSteps() []connector.AuthStep {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.authSteps)
}

// ConversationSnapshot is a conversation's state after every update a
// sink has seen, independent of how many arrived or in what order:
// title, pinned and archived flags, the service's own unread count, and
// which message remote ids are still present once deletes are applied.
// Scenario's Reorder and DuplicateDelivery checks compare these across
// differently ordered or repeated deliveries of the same events.
type ConversationSnapshot struct {
	Title            string
	Pinned, Archived bool
	Unread           int
	MessageRemoteIDs []string
}

// Snapshot returns the final state of every conversation this sink has
// seen, keyed by remote id.
func (s *Sink) Snapshot() map[string]ConversationSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	remotes := map[string]bool{}
	for remote := range s.conversations {
		remotes[remote] = true
	}
	for remote := range s.history {
		remotes[remote] = true
	}
	for remote := range s.live {
		remotes[remote] = true
	}

	out := make(map[string]ConversationSnapshot, len(remotes))
	for remote := range remotes {
		out[remote] = s.snapshotOf(remote)
	}

	return out
}

// snapshotOf builds remote's snapshot; the caller holds s.mu.
func (s *Sink) snapshotOf(remote string) ConversationSnapshot {
	present := map[string]bool{}
	for _, m := range s.history[remote] {
		present[m.RemoteID] = true
	}
	for _, m := range s.live[remote] {
		present[m.RemoteID] = true
	}
	for id := range s.deleted[remote] {
		delete(present, id)
	}

	ids := make([]string, 0, len(present))
	for id := range present {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	flags := s.organized[remote]

	return ConversationSnapshot{
		Title: s.conversations[remote].Title, Pinned: flags.pinned, Archived: flags.archived,
		Unread: s.unread[remote], MessageRemoteIDs: ids,
	}
}
