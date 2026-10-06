package telegram

import (
	"context"
	"fmt"
	"sync"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fakeTelegram answers API requests with canned replies, keyed by the
// request's Go type, and records the requests it was sent.
type fakeTelegram struct {
	mu       sync.Mutex
	replies  map[string]bin.Encoder
	requests []bin.Encoder
}

// newFakeTelegram returns a fake with no replies; unanswered requests fail.
func newFakeTelegram() *fakeTelegram {
	return &fakeTelegram{replies: map[string]bin.Encoder{}}
}

// reply sets the answer to every request of the same type as request.
func (f *fakeTelegram) reply(request, response bin.Encoder) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.replies[fmt.Sprintf("%T", request)] = response
}

// Invoke encodes the canned reply and decodes it into output, as the
// network layer would.
func (f *fakeTelegram) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, input)
	response, ok := f.replies[fmt.Sprintf("%T", input)]
	if !ok {
		return fmt.Errorf("fake telegram: no reply for %T", input)
	}

	var b bin.Buffer
	if err := response.Encode(&b); err != nil {
		return err
	}

	return output.Decode(&b)
}

// sent returns the requests received, in order.
func (f *fakeTelegram) sent() []bin.Encoder {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]bin.Encoder(nil), f.requests...)
}

// recordingSink records what a connector reports, one line per event.
type recordingSink struct {
	mu     sync.Mutex
	events []string
}

var _ connector.Sink = (*recordingSink)(nil)

// record adds one event line.
func (s *recordingSink) record(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, fmt.Sprintf(format, args...))
}

// lines returns the events recorded so far.
func (s *recordingSink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.events...)
}

// AccountStatus records a status change.
func (s *recordingSink) AccountStatus(_ context.Context, accountID, status, _ string) {
	s.record("status %s %s", accountID, status)
}

// Contact records a contact.
func (s *recordingSink) Contact(_ context.Context, c domain.Contact) {
	s.record("contact %s %s", c.RemoteID, c.Name)
}

// Conversation records a conversation.
func (s *recordingSink) Conversation(_ context.Context, c domain.Conversation) {
	s.record("conversation %s %s", c.RemoteID, c.Title)
}

// Incoming records a new message.
func (s *recordingSink) Incoming(_ context.Context, _, remote string, m domain.Message) {
	s.record("incoming %s %s %s", remote, m.RemoteID, m.Text)
}

// History records a past message.
func (s *recordingSink) History(_ context.Context, _, remote string, m domain.Message) {
	s.record("history %s %s %s", remote, m.RemoteID, m.Text)
}

// OutgoingStatus records a change to a sent message.
func (s *recordingSink) OutgoingStatus(_ context.Context, localID, remoteID, status string) {
	s.record("outgoing %s %s %s", localID, remoteID, status)
}

// Typing records someone starting or stopping typing.
func (s *recordingSink) Typing(_ context.Context, _, remote, _ string, active bool) {
	s.record("typing %s %t", remote, active)
}

// AuthStep records a sign-in step.
func (s *recordingSink) AuthStep(_ context.Context, _ string, step connector.AuthStep) {
	s.record("auth %s", step.Kind)
}

// connectedTo returns a connector already signed in to the fake.
func connectedTo(f *fakeTelegram, sink connector.Sink) *Connector {
	c := New(domain.Account{ID: "tg", Service: domain.ServiceTelegram}, "")
	c.connected(tg.NewClient(f), sink)

	return c
}
