package demo

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Delays that make the demo feel like a real service.
const (
	failDelay      = 800 * time.Millisecond
	sentDelay      = 250 * time.Millisecond
	deliveredDelay = 900 * time.Millisecond
	readDelay      = 2500 * time.Millisecond
	typingDelay    = 3 * time.Second
	replyDelay     = 4500 * time.Millisecond
	minChatter     = 30 * time.Second
	maxChatter     = 90 * time.Second
)

// Connector is one scripted demo account.
type Connector struct {
	script  accountScript
	chatter *atomic.Bool
	rng     *rand.Rand // used only by the chatter goroutine
	seq     atomic.Uint64

	mu       sync.Mutex
	run      *run // nil when not running
	attempts map[string]int
	replies  map[string]int
}

// run is one Run call: where updates go and how work is scheduled.
type run struct {
	sink  connector.Sink
	later func(delay time.Duration, report func(context.Context, connector.Sink))
}

// newConnector creates a stopped connector for script.
func newConnector(script accountScript, chatter *atomic.Bool, rng *rand.Rand) *Connector {
	return &Connector{
		script: script, chatter: chatter, rng: rng,
		attempts: make(map[string]int), replies: make(map[string]int),
	}
}

// Account describes the demo account.
func (c *Connector) Account() domain.Account {
	return c.script.account
}

// Run seeds the account's history, connects after a short delay and then
// plays scripted activity until ctx is cancelled.
func (c *Connector) Run(ctx context.Context, sink connector.Sink) error {
	var wg sync.WaitGroup
	if err := c.start(ctx, sink, &wg); err != nil {
		return err
	}

	id := c.script.account.ID
	sink.AccountStatus(ctx, id, domain.AccountConnecting, "")
	c.seed(ctx, sink)
	c.after(c.script.connectDelay, func(ctx context.Context, s connector.Sink) {
		s.AccountStatus(ctx, id, domain.AccountConnected, "")
	})
	wg.Go(func() { c.chat(ctx, sink) })

	<-ctx.Done()
	c.stop()
	wg.Wait()

	// The run is over, but the final status must still be recorded.
	sink.AccountStatus(context.WithoutCancel(ctx), id, domain.AccountOffline, "")

	return nil
}

// start records the running state. Work scheduled with after runs on wg and
// stops when ctx is cancelled.
func (c *Connector) start(ctx context.Context, sink connector.Sink, wg *sync.WaitGroup) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.run != nil {
		return ErrAlreadyRunning
	}

	later := func(delay time.Duration, report func(context.Context, connector.Sink)) {
		wg.Go(func() {
			if sleep(ctx, delay) {
				report(ctx, sink)
			}
		})
	}
	c.run = &run{sink: sink, later: later}

	return nil
}

// stop clears the running state so no new work is scheduled.
func (c *Connector) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.run = nil
}

// current returns the running state, or ErrNotRunning.
func (c *Connector) current() (*run, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.run == nil {
		return nil, ErrNotRunning
	}

	return c.run, nil
}

// after reports to the sink after delay, unless the run stops first. The
// lock keeps scheduling and stop ordered, so nothing starts after Run has
// begun waiting for its work to finish.
func (c *Connector) after(delay time.Duration, report func(context.Context, connector.Sink)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.run != nil {
		c.run.later(delay, report)
	}
}

// seed reports the account's contacts, conversations and history. Remote
// ids are stable, so seeding again after a reconnect changes nothing.
func (c *Connector) seed(ctx context.Context, sink connector.Sink) {
	id := c.script.account.ID
	for _, contact := range c.script.contacts {
		contact.AccountID = id
		sink.Contact(ctx, contact)
	}

	now := time.Now()
	for _, conv := range c.script.conversations {
		sink.Conversation(ctx, conv.conversation(id))

		for _, m := range conv.history(now) {
			sink.History(ctx, id, conv.remoteID, m)
		}
	}
}

// Send plays out delivery of an outgoing message: sent, delivered and, in
// direct chats, read followed by a typed reply. A flaky conversation fails
// each message's first attempt.
func (c *Connector) Send(ctx context.Context, conv domain.Conversation, m domain.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := c.current(); err != nil {
		return err
	}

	script, _ := c.script.find(conv.RemoteID)
	if script.flaky && c.attempt(m.ID) == 1 {
		c.after(failDelay, func(ctx context.Context, s connector.Sink) {
			s.OutgoingStatus(ctx, m.ID, "", domain.StatusFailed)
		})

		return nil
	}

	direct := conv.Kind == domain.KindDirect
	c.deliver(m.ID, direct)

	if direct && len(script.replies) > 0 {
		c.replyTo(script, m.ID)
	}

	return nil
}

// attempt counts and returns the send attempts for a message.
func (c *Connector) attempt(messageID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.attempts[messageID]++

	return c.attempts[messageID]
}

// receipt is a delivery status reported after a delay.
type receipt struct {
	delay  time.Duration
	status string
}

// deliver schedules the sent and delivered receipts for a message, and the
// read receipt in a direct chat. Groups have no single reader.
func (c *Connector) deliver(messageID string, direct bool) {
	remoteID := "demo-" + messageID
	receipts := []receipt{{sentDelay, domain.StatusSent}, {deliveredDelay, domain.StatusDelivered}}
	if direct {
		receipts = append(receipts, receipt{readDelay, domain.StatusRead})
	}

	for _, step := range receipts {
		c.after(step.delay, func(ctx context.Context, s connector.Sink) {
			s.OutgoingStatus(ctx, messageID, remoteID, step.status)
		})
	}
}

// replyTo shows the other person typing, then delivers their reply.
func (c *Connector) replyTo(script conversationScript, sentID string) {
	id := c.script.account.ID
	c.after(typingDelay, func(ctx context.Context, s connector.Sink) {
		s.Typing(ctx, id, script.remoteID, script.title, true)
	})

	c.after(replyDelay, func(ctx context.Context, s connector.Sink) {
		s.Typing(ctx, id, script.remoteID, script.title, false)
		s.Incoming(ctx, id, script.remoteID, script.reply(c.nextReply(script.remoteID), "demo-reply-"+sentID, time.Now()))
	})
}

// nextReply returns how many replies the conversation has had, then counts
// one more.
func (c *Connector) nextReply(remoteID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := c.replies[remoteID]
	c.replies[remoteID]++

	return n
}

// MarkRead accepts read receipts while the connector is running.
func (c *Connector) MarkRead(ctx context.Context, _ domain.Conversation) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := c.current()

	return err
}

// inject delivers a scripted message into a conversation now.
func (c *Connector) inject(ctx context.Context, remoteID string) (domain.Message, error) {
	r, err := c.current()
	if err != nil {
		return domain.Message{}, err
	}

	script, _ := c.script.find(remoteID)
	m := domain.Message{
		RemoteID:   fmt.Sprintf("demo-injected-%d", c.seq.Add(1)),
		SenderID:   script.remoteID,
		SenderName: script.title,
		Text:       "A scripted demo message arrived.",
		Status:     domain.StatusReceived,
		Created:    time.Now().UnixMilli(),
	}
	r.sink.Incoming(ctx, c.script.account.ID, remoteID, m)

	return m, nil
}

// chat delivers a random scripted reply at random intervals while chatter
// is on, until ctx is cancelled.
func (c *Connector) chat(ctx context.Context, sink connector.Sink) {
	chatty := c.script.chatty()
	if len(chatty) == 0 {
		return
	}

	for sleep(ctx, c.chatterDelay()) {
		if !c.chatter.Load() {
			continue
		}

		script := chatty[c.rng.IntN(len(chatty))]
		m := script.reply(c.rng.IntN(len(script.replies)), fmt.Sprintf("chatter-%d", c.seq.Add(1)), time.Now())
		sink.Incoming(ctx, c.script.account.ID, script.remoteID, m)
	}
}

// chatterDelay picks the wait before the next chatter message.
func (c *Connector) chatterDelay() time.Duration {
	return minChatter + time.Duration(c.rng.Int64N(int64(maxChatter-minChatter)+1))
}

// find returns the conversation script with the given remote id.
func (a accountScript) find(remoteID string) (conversationScript, bool) {
	for _, conv := range a.conversations {
		if conv.remoteID == remoteID {
			return conv, true
		}
	}

	return conversationScript{}, false
}

// chatty returns the conversations that have scripted replies.
func (a accountScript) chatty() []conversationScript {
	var out []conversationScript
	for _, conv := range a.conversations {
		if len(conv.replies) > 0 {
			out = append(out, conv)
		}
	}

	return out
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
