package app_test

// scenario_test.go reproduces, at the app layer, the two classes of bug
// a unit test with one tidy input never sees: the helper process
// restarting mid-session, and a connector's updates for one
// conversation arriving in a different relative order than the happy
// path assumes. Both run Ingest and Commands together over a real
// SQLite store, the way the helper actually wires them in main.go.

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
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

		db1, err := store.Open(ctx, path)
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
		db2, err := store.Open(ctx, path)
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
