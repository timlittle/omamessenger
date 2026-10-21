package app_test

// scenario_test.go reproduces, at the app layer, the two classes of bug
// a unit test with one tidy input never sees: the helper process
// restarting mid-session, and a connector's updates for one
// conversation arriving in a different relative order than the happy
// path assumes. Both run Ingest and Commands together over a real
// SQLite store, the way the helper actually wires them in main.go.

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// TestIngest_SurvivesAHelperRestart reproduces the helper process
// restarting mid-session: a second Store and Ingest, opened fresh over
// the same on-disk database, must see every conversation, message, pin
// and unread count the first instance left behind, and must still be
// able to receive new messages and report them read, using nothing
// kept only in the first instance's memory.
func TestIngest_SurvivesAHelperRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		path := filepath.Join(t.TempDir(), "messages.db")

		db1, err := store.Open(ctx, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := db1.UpsertAccount(ctx, domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}); err != nil {
			t.Fatal(err)
		}

		commands1, ingest1, _, _ := appOver(t, db1)
		ingest1.Conversation(ctx, domain.Conversation{AccountID: "wa", RemoteID: "r-chat", Title: "Alex", Kind: domain.KindDirect})
		ingest1.Incoming(ctx, "wa", "r-chat", incoming("in-1", "first"))
		ingest1.Incoming(ctx, "wa", "r-chat", incoming("in-2", "second"))

		before, err := commands1.Conversations(ctx, "")
		if err != nil || len(before) != 1 {
			t.Fatalf("conversations before restart = %+v, %v", before, err)
		}
		chatID := before[0].ID
		if _, err := commands1.SetPinned(ctx, chatID, true); err != nil {
			t.Fatal(err)
		}

		if err := db1.Close(); err != nil {
			t.Fatal(err)
		}

		// The helper restarts: a brand new Store and Ingest over the
		// same database, with nothing carried over from the first
		// instance but what it wrote to disk.
		db2, err := store.Open(ctx, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db2.Close() })
		commands2, ingest2, dispatcher2, _ := appOver(t, db2)

		after, err := commands2.Conversations(ctx, "")
		if err != nil || len(after) != 1 {
			t.Fatalf("conversations after restart = %+v, %v", after, err)
		}
		got := after[0]
		if !got.Pinned {
			t.Error("the pin set before the restart was lost")
		}
		if got.Unread != 2 {
			t.Errorf("unread after restart = %d, want 2", got.Unread)
		}

		messages, _, _, err := commands2.Messages(ctx, got.ID, "", 10)
		if err != nil || len(messages) != 2 {
			t.Fatalf("messages after restart = %+v, %v, want the 2 messages from before it", messages, err)
		}

		// A message arriving after the restart must still land in the
		// same, already-known conversation and still count towards its
		// unread total.
		ingest2.Incoming(ctx, "wa", "r-chat", incoming("in-3", "third"))
		if after, err := db2.Conversation(ctx, got.ID); err != nil || after.Unread != 3 {
			t.Errorf("conversation after restart and a new message = %+v, %v, want unread 3", after, err)
		}

		// Focusing the conversation after the restart must still report
		// it read to the service once the debounce elapses, proving
		// MarkRead's own data (see markread.go in the whatsapp package)
		// needs nothing that lived only in the first Ingest's memory.
		if err := commands2.SetFocus(ctx, got.ID, true); err != nil {
			t.Fatal(err)
		}
		ingest2.Incoming(ctx, "wa", "r-chat", incoming("in-4", "fourth"))

		time.Sleep(time.Second)
		synctest.Wait()

		if got := dispatcher2.read; !slices.Equal(got, []string{chatID}) {
			t.Errorf("read receipts after restart = %v, want one report for %s", got, chatID)
		}
	})
}

// TestIngest_HandlesReorderedUpdates reproduces a connector's updates
// for one conversation arriving in a different relative order than the
// happy path assumes: two incoming messages and a pin, delivered in
// several different orders, after the conversation itself is already
// known. Every ordering must leave the conversation pinned with both
// messages counted, regardless of where the pin fell among them.
func TestIngest_HandlesReorderedUpdates(t *testing.T) {
	t.Parallel()

	type step func(ctx context.Context, in *app.Ingest)

	incoming1 := func(ctx context.Context, in *app.Ingest) { in.Incoming(ctx, "wa", "r-chat", incoming("m1", "hi")) }
	incoming2 := func(ctx context.Context, in *app.Ingest) { in.Incoming(ctx, "wa", "r-chat", incoming("m2", "there")) }
	organize := func(ctx context.Context, in *app.Ingest) { in.Organized(ctx, "wa", "r-chat", true, false) }

	orderings := map[string][]step{
		"pin after both messages":      {incoming1, incoming2, organize},
		"pin before either message":    {organize, incoming1, incoming2},
		"pin between the two messages": {incoming1, organize, incoming2},
		"fully reversed":               {organize, incoming2, incoming1},
	}

	for name, steps := range orderings {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, false)
			ctx := t.Context()
			chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

			for _, step := range steps {
				step(ctx, f.ingest)
			}

			got, err := f.store.Conversation(ctx, chat.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Pinned {
				t.Error("pinned state lost by this ordering, want it applied regardless of when it arrived")
			}
			if got.Unread != 2 {
				t.Errorf("unread = %d, want 2 regardless of ordering", got.Unread)
			}

			messages, _, err := f.store.Messages(ctx, chat.ID, "", 10)
			if err != nil || len(messages) != 2 {
				t.Fatalf("messages = %+v, %v, want both messages regardless of ordering", messages, err)
			}
		})
	}
}

// twoAccountFixture wires a real store and a real connector.Manager
// around two of the fake package's own Telegram accounts, "tg-personal"
// and "tg-work", the way main.go wires the helper, so a test can prove
// two accounts of the same service stay independent through the public
// Commands and Ingest API rather than a stand-in dispatcher.
type twoAccountFixture struct {
	db       *store.Store
	commands *app.Commands
	manager  *connector.Manager
	suite    *fake.Suite
}

// newTwoAccountFixture starts both accounts and waits, inside the
// caller's synctest bubble, for them to seed their conversations.
func newTwoAccountFixture(t *testing.T) *twoAccountFixture {
	t.Helper()

	ctx := t.Context()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "messages.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	suite := fake.New()
	manager, err := connector.NewManager(connectorNamed(t, suite, "tg-personal"), connectorNamed(t, suite, "tg-work"))
	if err != nil {
		t.Fatal(err)
	}

	deps := app.Deps{
		Store: db, Dispatcher: manager, Notifier: &fakeNotifier{}, Publisher: &fakePublisher{},
		Cache:     cache.New(filepath.Join(t.TempDir(), "media"), 1<<20),
		Outgoing:  cache.NewOutgoing(filepath.Join(t.TempDir(), "outgoing"), 24*time.Hour, 1<<30, storeExists(db)),
		Clipboard: &fakeClipboard{}, Logger: &fakeLogger{},
	}
	commands, ingest := app.New(deps)

	runCtx, cancel := context.WithCancel(ctx)
	if err := manager.Start(runCtx, db, ingest); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); manager.Wait() })
	synctest.Wait()

	return &twoAccountFixture{db: db, commands: commands, manager: manager, suite: suite}
}

// connectorNamed returns the suite's connector for accountID, failing the
// test if the fake package ever drops or renames it.
func connectorNamed(t *testing.T, suite *fake.Suite, accountID string) connector.Connector {
	t.Helper()

	for _, c := range suite.Connectors() {
		if c.Account().ID == accountID {
			return c
		}
	}

	t.Fatalf("fake suite has no account %q", accountID)

	return nil
}

// conversationByRemote returns the one conversation whose remote id is
// remoteID, failing the test if seeding never reported it.
func conversationByRemote(t *testing.T, convs []domain.Conversation, remoteID string) domain.Conversation {
	t.Helper()

	for _, c := range convs {
		if c.RemoteID == remoteID {
			return c
		}
	}

	t.Fatalf("no seeded conversation with remote id %q among %+v", remoteID, convs)

	return domain.Conversation{}
}

// TestTwoAccounts_OfTheSameServiceStayAttributedIndependently proves
// issue #12's premise: adding two accounts of the same service (here,
// two Telegram accounts) keeps each one's conversations attributed to
// its own account id, never merged or confused with the other's.
func TestTwoAccounts_OfTheSameServiceStayAttributedIndependently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		f := newTwoAccountFixture(t)

		accounts, err := f.commands.Accounts(ctx)
		if err != nil || len(accounts) != 2 {
			t.Fatalf("accounts = %+v, %v, want both", accounts, err)
		}
		for _, a := range accounts {
			if a.Service != domain.ServiceTelegram {
				t.Errorf("account %+v, want service %q", a, domain.ServiceTelegram)
			}
		}

		convs, err := f.commands.Conversations(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		nadia := conversationByRemote(t, convs, "tg:nadia")
		jordan := conversationByRemote(t, convs, "tg:jordan-manager")
		if nadia.AccountID != "tg-personal" {
			t.Errorf("tg:nadia attributed to %q, want tg-personal", nadia.AccountID)
		}
		if jordan.AccountID != "tg-work" {
			t.Errorf("tg:jordan-manager attributed to %q, want tg-work", jordan.AccountID)
		}
	})
}

// TestTwoAccounts_OfTheSameServiceReceiveIndependently proves an
// incoming message delivered to one account of a service never changes
// the other account's own conversation, even though both share a
// service and a connector type.
func TestTwoAccounts_OfTheSameServiceReceiveIndependently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		f := newTwoAccountFixture(t)

		convs, err := f.commands.Conversations(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		nadia := conversationByRemote(t, convs, "tg:nadia")
		jordanBefore := conversationByRemote(t, convs, "tg:jordan-manager")

		if _, err := f.suite.Inject(ctx, "tg:nadia"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		afterNadia, err := f.db.Conversation(ctx, nadia.ID)
		if err != nil || afterNadia.Unread != nadia.Unread+1 {
			t.Fatalf("tg-personal's conversation after its own incoming message = %+v, %v, want unread %d", afterNadia, err, nadia.Unread+1)
		}

		afterJordan, err := f.db.Conversation(ctx, jordanBefore.ID)
		if err != nil || afterJordan.Unread != jordanBefore.Unread {
			t.Errorf("tg-work's conversation changed by a message delivered to tg-personal: %+v, %v", afterJordan, err)
		}
	})
}

// TestTwoAccounts_OfTheSameServiceSendIndependently proves a message
// sent through one account of a service reaches only that account's own
// connector, is delivered and read on its own, and never touches the
// other account's conversation.
func TestTwoAccounts_OfTheSameServiceSendIndependently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		f := newTwoAccountFixture(t)

		convs, err := f.commands.Conversations(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		nadia := conversationByRemote(t, convs, "tg:nadia")
		jordan := conversationByRemote(t, convs, "tg:jordan-manager")

		if _, err := f.commands.Send(ctx, jordan.ID, "hello from work", app.SendOptions{}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()

		sent, _, _, err := f.commands.Messages(ctx, jordan.ID, "", 10)
		if err != nil || len(sent) == 0 {
			t.Fatalf("messages on tg-work = %+v, %v", sent, err)
		}
		if last := sent[len(sent)-1]; last.Text != "hello from work" || last.Status != domain.StatusRead {
			t.Fatalf("message sent on tg-work = %+v, want it read", last)
		}

		untouched, err := f.db.Conversation(ctx, nadia.ID)
		if err != nil || untouched.Unread != nadia.Unread {
			t.Errorf("sending on tg-work changed tg-personal's conversation: %+v, %v", untouched, err)
		}
	})
}

// TestTwoAccounts_RemovingOneLeavesTheOtherRunning proves removing one
// account of a service deletes only its own data and connector, leaving
// a second account of the same service running and fully intact.
func TestTwoAccounts_RemovingOneLeavesTheOtherRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		f := newTwoAccountFixture(t)

		convs, err := f.commands.Conversations(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		nadia := conversationByRemote(t, convs, "tg:nadia")
		jordan := conversationByRemote(t, convs, "tg:jordan-manager")

		if err := f.manager.Remove(ctx, "tg-work"); err != nil {
			t.Fatal(err)
		}
		if err := f.db.DeleteAccount(ctx, "tg-work"); err != nil {
			t.Fatal(err)
		}

		remaining, err := f.commands.Accounts(ctx)
		if err != nil || len(remaining) != 1 || remaining[0].ID != "tg-personal" {
			t.Fatalf("accounts after removing tg-work = %+v, %v, want only tg-personal", remaining, err)
		}

		if _, err := f.db.Conversation(ctx, jordan.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("tg-work's conversation survived its account's removal: %v", err)
		}

		survivor, err := f.db.Conversation(ctx, nadia.ID)
		if err != nil || survivor.AccountID != "tg-personal" {
			t.Fatalf("tg-personal's conversation after removing tg-work = %+v, %v", survivor, err)
		}

		if _, err := f.suite.Inject(ctx, "tg:nadia"); err != nil {
			t.Errorf("tg-personal stopped working after its sibling account was removed: %v", err)
		}
	})
}
