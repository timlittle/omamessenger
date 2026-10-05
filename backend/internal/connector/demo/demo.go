// Package demo provides deterministic scripted connectors for offline UI and
// end-to-end development.
package demo

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var errNotRunning = errors.New("demo connector is not running")
var errUnknownConversation = errors.New("demo conversation not found")

// suite is the state shared by the demo connectors of one helper.
type suite struct {
	clock   connector.Clock
	rng     *rand.Rand
	rngMu   sync.Mutex
	chatter atomic.Bool
}

type demoConnector struct {
	suite   *suite
	script  accountScript
	mu      sync.Mutex
	sink    connector.Sink
	running bool
	timers  map[uint64]func() bool
	timerID uint64
	attempt map[string]int
	replies map[string]int
	inject  atomic.Uint64
}

// New creates one Connector per demo account. A nil Clock or RNG is replaced
// with a real clock or a time-seeded random source.
func New(clock connector.Clock, rng *rand.Rand, chatter bool) []connector.Connector {
	if clock == nil {
		clock = connector.RealClock{}
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	shared := &suite{clock: clock, rng: rng}
	shared.chatter.Store(chatter)
	connectors := make([]connector.Connector, 0, len(scripts))
	for _, script := range scripts {
		connectors = append(connectors, &demoConnector{
			suite: shared, script: script,
			attempt: make(map[string]int), replies: make(map[string]int), timers: make(map[uint64]func() bool),
		})
	}
	return connectors
}

func (d *demoConnector) Account() domain.Account { return d.script.account }

func (d *demoConnector) Run(ctx context.Context, sink connector.Sink) error {
	if ctx == nil || sink == nil {
		return errors.New("demo connector requires a context and sink")
	}
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return errors.New("demo connector is already running")
	}
	d.sink = sink
	d.running = true
	d.mu.Unlock()

	sink.AccountStatus(d.script.account.ID, domain.AccountConnecting, "")
	d.seed(sink)
	connectedAfter := 600 * time.Millisecond
	if d.script.account.ID == "tg-work" {
		connectedAfter = 2 * time.Second
	}
	d.after(connectedAfter, func(s connector.Sink) {
		s.AccountStatus(d.script.account.ID, domain.AccountConnected, "")
	})
	d.scheduleChatter()

	<-ctx.Done()
	d.stop()
	sink.AccountStatus(d.script.account.ID, domain.AccountOffline, "")
	return nil
}

func (d *demoConnector) seed(sink connector.Sink) {
	now := d.suite.clock.Now()
	for _, contact := range d.script.contacts {
		contact.AccountID = d.script.account.ID
		sink.Contact(contact)
	}
	for _, conversation := range d.script.conversations {
		sink.Conversation(domain.Conversation{
			AccountID: d.script.account.ID, RemoteID: conversation.remoteID,
			Kind: conversation.kind, Title: conversation.title, Members: conversation.members,
			Muted: conversation.muted,
		})
		for _, message := range messagesFor(conversation, now) {
			deliverHistory(sink, d.script.account.ID, conversation.remoteID, message)
		}
	}
}

func deliverHistory(sink connector.Sink, accountID, remoteID string, message domain.Message) {
	if history, ok := sink.(connector.HistorySink); ok {
		history.History(accountID, remoteID, message)
		return
	}
	sink.Incoming(accountID, remoteID, message)
}

func (d *demoConnector) Send(ctx context.Context, conversation domain.Conversation, message domain.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.active() {
		return errNotRunning
	}
	d.mu.Lock()
	d.attempt[message.ID]++
	attempt := d.attempt[message.ID]
	d.mu.Unlock()
	if conversation.Title == "Sam (spotty signal)" && attempt == 1 {
		d.after(800*time.Millisecond, func(s connector.Sink) {
			s.OutgoingStatus(message.ID, "", domain.StatusFailed)
		})
		return nil
	}

	remoteID := "demo-" + message.ID
	d.after(250*time.Millisecond, func(s connector.Sink) {
		s.OutgoingStatus(message.ID, remoteID, domain.StatusSent)
	})
	d.after(900*time.Millisecond, func(s connector.Sink) {
		s.OutgoingStatus(message.ID, remoteID, domain.StatusDelivered)
	})
	if conversation.Kind == domain.KindDirect {
		d.after(2500*time.Millisecond, func(s connector.Sink) {
			s.OutgoingStatus(message.ID, remoteID, domain.StatusRead)
		})
		remote := d.conversation(conversation.RemoteID)
		if remote != nil && len(remote.replies) > 0 {
			name := remote.title
			d.after(3*time.Second, func(s connector.Sink) {
				s.Typing(d.script.account.ID, remote.remoteID, name, true)
			})
			d.after(4500*time.Millisecond, func(s connector.Sink) {
				s.Typing(d.script.account.ID, remote.remoteID, name, false)
				s.Incoming(d.script.account.ID, remote.remoteID, d.reply(remote, message.ID))
			})
		}
	}
	return nil
}

func (d *demoConnector) MarkRead(ctx context.Context, conversation domain.Conversation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !d.active() {
		return errNotRunning
	}
	return nil
}

func (d *demoConnector) Inject(conversationRemoteID string) (domain.Message, error) {
	conversation := d.conversation(conversationRemoteID)
	if conversation == nil {
		return domain.Message{}, errUnknownConversation
	}
	sink, ok := d.currentSink()
	if !ok {
		return domain.Message{}, errNotRunning
	}
	m := domain.Message{
		RemoteID: fmt.Sprintf("demo-injected-%d", d.inject.Add(1)),
		SenderID: conversation.remoteID, SenderName: conversation.title,
		Text: "A scripted demo message arrived.", Status: domain.StatusReceived,
		Created: d.suite.clock.Now().UnixMilli(),
	}
	sink.Incoming(d.script.account.ID, conversation.remoteID, m)
	return m, nil
}

func (d *demoConnector) conversation(remoteID string) *conversationScript {
	for i := range d.script.conversations {
		if d.script.conversations[i].remoteID == remoteID {
			return &d.script.conversations[i]
		}
	}
	return nil
}

func (d *demoConnector) reply(conversation *conversationScript, sentID string) domain.Message {
	d.mu.Lock()
	index := d.replies[conversation.remoteID]
	d.replies[conversation.remoteID]++
	d.mu.Unlock()
	text := conversation.replies[index%len(conversation.replies)]
	sender := conversation.title
	if len(conversation.groupSenders) > 0 {
		sender = conversation.groupSenders[index%len(conversation.groupSenders)]
	}
	return domain.Message{
		RemoteID: "demo-reply-" + sentID, SenderID: sender, SenderName: sender,
		Text: text, Status: domain.StatusReceived, Created: d.suite.clock.Now().UnixMilli(),
	}
}

func (d *demoConnector) scheduleChatter() {
	d.after(d.randomChatterDelay(), func(sink connector.Sink) {
		if d.suite.chatter.Load() {
			conversation := d.randomConversation()
			text := d.randomReply(conversation)
			sink.Incoming(d.script.account.ID, conversation.remoteID, domain.Message{
				RemoteID: fmt.Sprintf("chatter-%d", d.inject.Add(1)),
				SenderID: conversation.title, SenderName: conversation.title,
				Text: text, Status: domain.StatusReceived, Created: d.suite.clock.Now().UnixMilli(),
			})
		}
		d.scheduleChatter()
	})
}

func (d *demoConnector) randomChatterDelay() time.Duration {
	d.suite.rngMu.Lock()
	millis := 30_000 + d.suite.rng.Intn(60_001)
	d.suite.rngMu.Unlock()
	return time.Duration(millis) * time.Millisecond
}

func (d *demoConnector) randomConversation() *conversationScript {
	d.suite.rngMu.Lock()
	index := d.suite.rng.Intn(len(d.script.conversations))
	d.suite.rngMu.Unlock()
	return &d.script.conversations[index]
}

func (d *demoConnector) randomReply(conversation *conversationScript) string {
	if len(conversation.replies) == 0 {
		return "Just checking in"
	}
	d.suite.rngMu.Lock()
	index := d.suite.rng.Intn(len(conversation.replies))
	d.suite.rngMu.Unlock()
	return conversation.replies[index]
}

func (d *demoConnector) after(delay time.Duration, callback func(connector.Sink)) {
	d.mu.Lock()
	d.timerID++
	id := d.timerID
	d.mu.Unlock()
	stop := d.suite.clock.AfterFunc(delay, func() {
		d.mu.Lock()
		delete(d.timers, id)
		d.mu.Unlock()
		sink, active := d.currentSink()
		if active {
			callback(sink)
		}
	})
	d.mu.Lock()
	if d.running {
		d.timers[id] = stop
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()
	stop()
}

func (d *demoConnector) currentSink() (connector.Sink, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sink, d.running && d.sink != nil
}

func (d *demoConnector) active() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

func (d *demoConnector) stop() {
	d.mu.Lock()
	d.running = false
	d.sink = nil
	timers := d.timers
	d.timers = make(map[uint64]func() bool)
	d.mu.Unlock()
	for _, stop := range timers {
		stop()
	}
}

// Injector routes a demo injection to whichever demo account owns the remote
// conversation id.
type Injector struct{ connectors []*demoConnector }

func NewInjector(connectors ...connector.Connector) *Injector {
	injector := &Injector{}
	for _, candidate := range connectors {
		if demo, ok := candidate.(*demoConnector); ok {
			injector.connectors = append(injector.connectors, demo)
		}
	}
	return injector
}

// SetChatter enables or disables scripted incoming messages in these demo
// connectors. Their timers stay scheduled while disabled, so the setting can
// change without restarting the helper.
func (i *Injector) SetChatter(enabled bool) {
	for _, demo := range i.connectors {
		demo.suite.chatter.Store(enabled)
	}
}

func (i *Injector) Inject(conversationRemoteID string) (domain.Message, error) {
	for _, demo := range i.connectors {
		if demo.conversation(conversationRemoteID) != nil {
			return demo.Inject(conversationRemoteID)
		}
	}
	return domain.Message{}, errUnknownConversation
}
