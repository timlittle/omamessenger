package demo_test

import (
	"context"
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
	"github.com/timlittle/omamessenger/backend/internal/notify"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

type fixture struct {
	clock      *clocktest.Clock
	store      *store.Store
	app        *app.App
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
	a := app.New(db, nil, &notify.Recorder{}, clock)
	typing := make(chan bool, 8)
	a.Emit = func(name string, data any) {
		if name != "typing" {
			return
		}
		if value, ok := data.(struct {
			ConversationID string `json:"conversationId"`
			Name           string `json:"name"`
			Active         bool   `json:"active"`
		}); ok {
			typing <- value.Active
		}
	}
	a.Demo = true
	connectors := demo.New(clock, rand.New(rand.NewSource(42)), chatter)
	a.DemoInject = demo.NewInjector(connectors...)
	ctx, cancel := context.WithCancel(context.Background())
	f := &fixture{clock: clock, store: db, app: a, connectors: connectors, cancel: cancel, typing: typing}
	for _, c := range connectors {
		if err := db.UpsertAccount(c.Account()); err != nil {
			t.Fatal(err)
		}
		f.wg.Add(1)
		go func(c connector.Connector) { defer f.wg.Done(); _ = c.Run(ctx, a) }(c)
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
		go func() { defer wg.Done(); _ = script.Run(ctx, f.app) }()
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

	injected, err := f.app.Inject(context.Background(), app.InjectParams{ConversationID: "wa:mum"})
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
	demo.SetChatter(true)
	f.clock.Advance(2 * time.Minute)
	total := 0
	for _, c := range f.conversations(t) {
		total += c.Unread
	}
	if total <= 27 {
		t.Fatalf("enabled chatter did not add unread messages: %d", total)
	}
}
