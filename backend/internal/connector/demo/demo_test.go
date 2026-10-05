package demo_test

import (
	"context"
	"encoding/json"
	"math/rand"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest"
	"github.com/timlittle/omamessenger/backend/internal/connector/demo"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

type fixture struct {
	clock      *clocktest.Clock
	store      *store.Store
	commands   *app.Commands
	sink       *app.Ingest
	injector   *demo.Injector
	connectors []connector.Connector
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	typing     chan bool
}

func newFixture(t *testing.T, chatter bool) *fixture {
	t.Helper()
	clock := clocktest.New(time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC))
	db, err := store.Open(filepath.Join(t.TempDir(), "data", "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	typing := make(chan bool, 8)
	// Decode typing events through JSON, as the UI receives them.
	emit := func(name string, data any) {
		if name != "typing" {
			return
		}
		var event struct {
			Active bool `json:"active"`
		}
		if raw, err := json.Marshal(data); err == nil && json.Unmarshal(raw, &event) == nil {
			typing <- event.Active
		}
	}
	connectors := demo.New(clock, rand.New(rand.NewSource(42)), chatter)
	injector := demo.NewInjector(connectors...)
	commands, sink := app.New(app.Config{
		Repo: db, Notifier: silentNotifier{}, Clock: clock, Emit: emit,
		Demo: true, DemoInject: injector, SetChatter: injector.SetChatter,
	})
	ctx, cancel := context.WithCancel(context.Background())
	f := &fixture{clock: clock, store: db, commands: commands, sink: sink, injector: injector, connectors: connectors, cancel: cancel, typing: typing}
	for _, c := range connectors {
		if err := db.UpsertAccount(c.Account()); err != nil {
			t.Fatal(err)
		}
		f.wg.Add(1)
		go func(c connector.Connector) { defer f.wg.Done(); _ = c.Run(ctx, sink) }(c)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conversations, err := db.Conversations("")
		if err == nil && len(conversations) == 11 && clock.Pending() >= 6 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	conversations, err := db.Conversations("")
	if err != nil || len(conversations) != 11 {
		cancel()
		t.Fatalf("demo seed conversations=%d err=%v accounts=%#v chats=%#v", len(conversations), err, mustAccounts(t, db), conversations)
	}
	t.Cleanup(func() {
		cancel()
		// Run's only blocking operation is the context wait; cancellation makes
		// each connector finish before its store is closed.
		f.wg.Wait()
		_ = db.Close()
	})
	return f
}

func (f *fixture) conversations(t *testing.T) []domain.Conversation {
	t.Helper()
	cs, err := f.store.Conversations("")
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func find(t *testing.T, cs []domain.Conversation, title string) domain.Conversation {
	t.Helper()
	for _, c := range cs {
		if c.Title == title {
			return c
		}
	}
	t.Fatalf("conversation %q missing", title)
	return domain.Conversation{}
}

func TestSeededDemoAccountsUnreadAndIdempotency(t *testing.T) {
	f := newFixture(t, false)
	cs := f.conversations(t)
	want := map[string]int{"Mum": 0, "Climbing Crew": 3, "Alex Chen": 1, "Sam (spotty signal)": 0, "Flat 4B": 5, "Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)": 0, "Nadia": 2, "Omarchy Users": 12, "Saved Messages": 0, "Platform Team": 4, "Jordan (Manager)": 0}
	for title, unread := range want {
		if c := find(t, cs, title); c.Unread != unread {
			t.Errorf("%s unread=%d want %d", title, c.Unread, unread)
		}
	}
	if len(cs) != 11 {
		t.Fatalf("conversations=%d", len(cs))
	}
	before := append([]domain.Conversation(nil), cs...)
	for _, script := range demo.New(f.clock, rand.New(rand.NewSource(42)), false) {
		// Running a connector again simulates reconnect seeding. Stable remote
		// ids make it idempotent.
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = script.Run(ctx, f.sink) }()
		cancel()
		wg.Wait()
	}
	if got := f.conversations(t); !reflect.DeepEqual(got, before) {
		t.Fatalf("reseed changed conversation state\nbefore=%#v\nafter=%#v", before, got)
	}
	wantCounts := map[string]int{"Mum": 14, "Climbing Crew": 30, "Alex Chen": 6, "Sam (spotty signal)": 4, "Flat 4B": 12, "Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)": 2, "Nadia": 10, "Omarchy Users": 150, "Saved Messages": 5, "Platform Team": 20, "Jordan (Manager)": 8}
	for title, count := range wantCounts {
		c := find(t, f.conversations(t), title)
		msgs, _, err := f.store.Messages(c.ID, "", 200)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) != count {
			t.Errorf("%s message count=%d want=%d", title, len(msgs), count)
		}
	}
}

func TestStatusSendFailureRetryReplyTypingAndInjection(t *testing.T) {
	f := newFixture(t, false)
	f.clock.Advance(599 * time.Millisecond)
	for _, account := range mustAccounts(t, f.store) {
		if account.Status != domain.AccountConnecting {
			t.Errorf("%s at 599ms status=%s", account.ID, account.Status)
		}
	}
	f.clock.Advance(time.Millisecond)
	for _, account := range mustAccounts(t, f.store) {
		want := domain.AccountConnected
		if account.ID == "tg-work" {
			want = domain.AccountConnecting
		}
		if account.Status != want {
			t.Errorf("%s at 600ms status=%s want=%s", account.ID, account.Status, want)
		}
	}
	f.clock.Advance(1400 * time.Millisecond)
	for _, account := range mustAccounts(t, f.store) {
		if account.Status != domain.AccountConnected {
			t.Errorf("%s at 2s status=%s", account.ID, account.Status)
		}
	}
	cs := f.conversations(t)
	sam := find(t, cs, "Sam (spotty signal)")
	c := f.connectors[0]
	m := domain.Message{ID: "outgoing-sam", ConversationID: sam.ID, Text: "hello", Outgoing: true, Status: domain.StatusPending, Created: f.clock.Now().UnixMilli()}
	if _, _, err := f.store.AddMessage(m); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), sam, m); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(800 * time.Millisecond)
	got, err := f.store.Message(m.ID)
	if err != nil || got.Status != domain.StatusFailed {
		t.Fatalf("first send=%#v err=%v", got, err)
	}
	if err := c.Send(context.Background(), sam, got); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(250 * time.Millisecond)
	got, _ = f.store.Message(m.ID)
	if got.Status != domain.StatusSent {
		t.Fatalf("retry status=%s", got.Status)
	}
	f.clock.Advance(650 * time.Millisecond)
	got, _ = f.store.Message(m.ID)
	if got.Status != domain.StatusDelivered {
		t.Fatalf("delivery status=%s", got.Status)
	}
	f.clock.Advance(1600 * time.Millisecond)
	got, _ = f.store.Message(m.ID)
	if got.Status != domain.StatusRead {
		t.Fatalf("read status=%s", got.Status)
	}

	alex := find(t, cs, "Alex Chen")
	alexMessage := domain.Message{ID: "outgoing-alex", ConversationID: alex.ID, Text: "hello", Outgoing: true, Status: domain.StatusPending, Created: f.clock.Now().UnixMilli()}
	if _, _, err := f.store.AddMessage(alexMessage); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), alex, alexMessage); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(4500 * time.Millisecond)
	alexMessages, _, err := f.store.Messages(alex.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	foundReply := false
	for _, msg := range alexMessages {
		if msg.RemoteID == "demo-reply-outgoing-alex" {
			foundReply = true
		}
	}
	if !foundReply {
		t.Error("direct message reply was not stored")
	}
	states := []bool{}
	for len(f.typing) > 0 {
		states = append(states, <-f.typing)
	}
	if len(states) < 2 || len(states)%2 != 0 {
		t.Errorf("typing events = %v, want balanced on/off pairs", states)
	} else {
		for i := range states {
			if states[i] != (i%2 == 0) {
				t.Errorf("typing events = %v, want alternating on/off", states)
				break
			}
		}
	}

	injected, err := f.commands.Inject(context.Background(), app.InjectParams{ConversationID: find(t, f.conversations(t), "Mum").ID})
	if err != nil || injected.ConversationID != find(t, f.conversations(t), "Mum").ID {
		t.Fatalf("inject=%#v err=%v", injected, err)
	}
}

func mustAccounts(t *testing.T, s *store.Store) []domain.Account {
	t.Helper()
	as, err := s.Accounts()
	if err != nil {
		t.Fatal(err)
	}
	return as
}

func TestChatterCanBeToggled(t *testing.T) {
	f := newFixture(t, false)
	before := len(f.conversations(t))
	f.clock.Advance(2 * time.Minute)
	if got := len(f.conversations(t)); got != before {
		t.Fatalf("disabled chatter altered conversation count: %d", got)
	}
	f.injector.SetChatter(true)
	f.clock.Advance(2 * time.Minute)
	total := 0
	for _, c := range f.conversations(t) {
		total += c.Unread
	}
	if total <= 27 {
		t.Fatalf("enabled chatter did not add unread messages: %d", total)
	}
}

// silentNotifier discards notifications so tests never run notify-send.
type silentNotifier struct{}

func (silentNotifier) Notify(string, string) {}

func TestNewDefaultsAndAccounts(t *testing.T) {
	connectors := demo.New(nil, nil, false)
	var ids []string
	for _, c := range connectors {
		ids = append(ids, c.Account().ID)
	}
	if want := []string{"wa-personal", "tg-personal", "tg-work"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("demo accounts = %v, want %v", ids, want)
	}
}

func TestStoppedConnectorRefusesWork(t *testing.T) {
	connectors := demo.New(clocktest.New(time.Unix(0, 0)), rand.New(rand.NewSource(1)), false)
	idle := connectors[0]
	conv := domain.Conversation{AccountID: "wa-personal", RemoteID: "wa:mum", Title: "Mum", Kind: domain.KindDirect}
	if err := idle.Send(context.Background(), conv, domain.Message{ID: "m"}); err == nil {
		t.Error("Send on a connector that is not running succeeded")
	}
	if err := idle.MarkRead(context.Background(), conv); err == nil {
		t.Error("MarkRead on a connector that is not running succeeded")
	}
	if _, err := demo.NewInjector(connectors...).Inject("wa:mum"); err == nil {
		t.Error("Inject on a connector that is not running succeeded")
	}
	var nilCtx context.Context
	if err := idle.Run(nilCtx, nil); err == nil {
		t.Error("Run without a context and sink succeeded")
	}
}

func TestRunningConnectorRules(t *testing.T) {
	f := newFixture(t, false)
	wa := f.connectors[0]
	conv := find(t, f.conversations(t), "Mum")
	if err := wa.MarkRead(context.Background(), conv); err != nil {
		t.Errorf("MarkRead on a running connector = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := wa.MarkRead(canceled, conv); err == nil {
		t.Error("MarkRead with a canceled context succeeded")
	}
	if err := wa.Send(canceled, conv, domain.Message{ID: "m"}); err == nil {
		t.Error("Send with a canceled context succeeded")
	}
	if err := wa.Run(context.Background(), f.sink); err == nil {
		t.Error("a second Run on a running connector succeeded")
	}
	if _, err := f.injector.Inject("not-a-demo-conversation"); err == nil {
		t.Error("Inject into an unknown conversation succeeded")
	}
}
