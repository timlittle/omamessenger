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
	mu        sync.Mutex
	replies   map[string]bin.Encoder
	sequences map[string][]bin.Encoder
	failures  map[string][]error
	requests  []bin.Encoder
}

// newFakeTelegram returns a fake with no replies; unanswered requests fail.
func newFakeTelegram() *fakeTelegram {
	return &fakeTelegram{
		replies:   map[string]bin.Encoder{},
		sequences: map[string][]bin.Encoder{},
		failures:  map[string][]error{},
	}
}

// replySequence queues responses to request's type, consumed one per
// call in order; once exhausted, Invoke falls back to whatever reply
// was set with reply. Use this where a single request type must answer
// differently across a few calls, such as Telegram reporting its
// difference too long once before a normal one completes the resync.
func (f *fakeTelegram) replySequence(request bin.Encoder, responses ...bin.Encoder) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := fmt.Sprintf("%T", request)
	f.sequences[key] = append(f.sequences[key], responses...)
}

// failNext makes the next requests of request's type fail with errs, in
// order, before the canned reply is used again.
func (f *fakeTelegram) failNext(request bin.Encoder, errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := fmt.Sprintf("%T", request)
	f.failures[key] = append(f.failures[key], errs...)
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
	key := fmt.Sprintf("%T", input)
	if errs := f.failures[key]; len(errs) > 0 {
		f.failures[key] = errs[1:]
		return errs[0]
	}

	response, ok := f.replies[key]
	if seq := f.sequences[key]; len(seq) > 0 {
		f.sequences[key] = seq[1:]
		response, ok = seq[0], true
	}
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

// connectedTo returns a connector already signed in to the fake.
func connectedTo(f *fakeTelegram, sink connector.Sink) *Connector {
	c := New(domain.Account{ID: "tg", Service: domain.ServiceTelegram}, "")
	c.connected(tg.NewClient(f), sink)

	return c
}

// connectedAndKnown returns a connector already signed in to the fake,
// with each of remoteIDs already learned, as most tests need before a
// call that resolves a peer from a remote id.
func connectedAndKnown(f *fakeTelegram, sink connector.Sink, remoteIDs ...string) *Connector {
	c := connectedTo(f, sink)
	for _, id := range remoteIDs {
		c.learn(id)
	}

	return c
}
