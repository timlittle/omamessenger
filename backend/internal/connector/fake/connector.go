package fake

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Delays that make the fake behave like a real service.
const (
	failDelay      = 800 * time.Millisecond
	sentDelay      = 250 * time.Millisecond
	deliveredDelay = 900 * time.Millisecond
	readDelay      = 2500 * time.Millisecond
)

// Connector is one scripted fake account.
type Connector struct {
	script accountScript
	seq    atomic.Uint64

	mu       sync.Mutex
	run      *run // nil when not running
	attempts map[string]int
	olderOut map[string]bool // conversations whose older history was delivered
}

var (
	_ connector.HistoryLoader = (*Connector)(nil)
	_ connector.MediaFetcher  = (*Connector)(nil)
	_ connector.Reactor       = (*Connector)(nil)
	_ connector.Voter         = (*Connector)(nil)
)

// run is one Run call: where updates go and how work is scheduled.
type run struct {
	sink  connector.Sink
	later func(delay time.Duration, report func(context.Context, connector.Sink))
}

// newConnector creates a stopped connector for script.
func newConnector(script accountScript) *Connector {
	return &Connector{script: script, attempts: make(map[string]int), olderOut: make(map[string]bool)}
}

// Account describes the fake account.
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

// seed reports the account's contacts, conversations, history and each
// conversation's unread count, the way the real Telegram connector's
// sync does: History never counts towards unread on its own, so the
// script's scripted count is reported afterwards, through Unread, to
// establish it.
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

		sink.Unread(ctx, id, conv.remoteID, conv.unread)
	}
}

// Send plays out delivery of an outgoing message: sent, delivered and, in
// direct chats, read. A flaky conversation fails each message's first
// attempt.
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

	c.deliver(m.ID, conv.Kind == domain.KindDirect)

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
	remoteID := "fake-" + messageID
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

// LoadOlder delivers a conversation's scripted older history the first
// time it is asked for, and nothing after that.
func (c *Connector) LoadOlder(ctx context.Context, conv domain.Conversation, _ string, _ int) (int, error) {
	r, err := c.current()
	if err != nil {
		return 0, err
	}

	script, _ := c.script.find(conv.RemoteID)
	if !c.firstOlderLoad(conv.RemoteID) {
		return 0, nil
	}

	older := script.olderHistory(time.Now())
	for _, m := range older {
		r.sink.History(ctx, c.script.account.ID, conv.RemoteID, m)
	}

	return len(older), nil
}

// FetchMedia writes a stand-in for a scripted photo or voice note to
// path: a small, solid-colour JPEG for a photo, or a short silent Opus
// clip for a voice note, each decoding like the real download would, so
// the UI has something real to show or play rather than falling back to
// its "unavailable" label. app.mediaFileName, the only place that
// names path, always gives a voice note's own file extension, so
// telling the two apart by path alone is enough: this connector never
// sees the message's media kind directly.
func (c *Connector) FetchMedia(_ context.Context, _ domain.Conversation, _, path string) error {
	if _, err := c.current(); err != nil {
		return err
	}

	data := placeholderPhoto()
	if isVoiceNotePath(path) {
		data = placeholderVoiceNote()
	}

	return os.WriteFile(path, data, 0o600)
}

// isVoiceNotePath reports whether path names a voice note rather than a
// photo, by its own file extension. The cache package fills a file
// through a temporary one, suffixed ".part", before renaming it into
// place, so that suffix is trimmed first: without it, every voice note
// would be mistaken for a plain file and get the photo placeholder
// instead, with the right ".ogg" name but the wrong bytes inside.
func isVoiceNotePath(path string) bool {
	path = strings.TrimSuffix(path, ".part")

	return strings.EqualFold(filepath.Ext(path), ".ogg")
}

// React sets or clears the user's own reaction and reports it back
// through the sink at once, the way Telegram echoes its own response
// rather than waiting for a matching live update.
func (c *Connector) React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error {
	r, err := c.current()
	if err != nil {
		return err
	}

	reactions := []domain.Reaction{}
	if emoji != "" {
		reactions = []domain.Reaction{{Emoji: emoji, Count: 1, Mine: true}}
	}

	r.sink.Reacted(ctx, c.script.account.ID, conv.RemoteID, messageRemoteID, reactions)
	return nil
}

// Vote records the user's choice and reports the poll's new tally back
// through the sink at once, the way React does for a reaction: every
// chosen option gets one vote, as the only voter a fake account ever
// has.
func (c *Connector) Vote(ctx context.Context, conv domain.Conversation, messageRemoteID string, optionIDs []string) error {
	r, err := c.current()
	if err != nil {
		return err
	}

	updater, ok := r.sink.(connector.PollUpdater)
	if !ok {
		return nil
	}

	options := make([]domain.PollOption, len(optionIDs))
	for i, id := range optionIDs {
		options[i] = domain.PollOption{ID: id, Votes: 1, Chosen: true}
	}

	updater.PollUpdated(ctx, c.script.account.ID, conv.RemoteID, messageRemoteID, domain.Poll{Options: options, TotalVoters: 1})

	return nil
}

// DeleteMessages accepts any delete while the connector is running.
// app.Commands.DeleteMessages removes the stored rows and publishes
// their removal itself once this returns, so there is nothing further
// to do here.
func (c *Connector) DeleteMessages(_ context.Context, _ domain.Conversation, _ []string, _ bool) error {
	_, err := c.current()

	return err
}

// firstOlderLoad records that a conversation's older history was asked
// for, reporting whether this is the first time.
func (c *Connector) firstOlderLoad(remoteID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	first := !c.olderOut[remoteID]
	c.olderOut[remoteID] = true

	return first
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
		RemoteID:   fmt.Sprintf("fake-injected-%d", c.seq.Add(1)),
		SenderID:   script.remoteID,
		SenderName: script.title,
		Text:       "A scripted message arrived.",
		Status:     domain.StatusReceived,
		Created:    time.Now().UnixMilli(),
	}
	r.sink.Incoming(ctx, c.script.account.ID, remoteID, m)

	return m, nil
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
