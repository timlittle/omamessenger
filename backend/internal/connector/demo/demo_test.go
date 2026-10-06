package demo_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector/demo"
)

func TestNew_CreatesTheDemoAccounts(t *testing.T) {
	t.Parallel()

	var ids []string
	for _, c := range demo.New(1, false).Connectors() {
		ids = append(ids, c.Account().ID)
	}

	if want := []string{"wa-personal", "tg-personal", "tg-work"}; !slices.Equal(ids, want) {
		t.Errorf("accounts = %v, want %v", ids, want)
	}
}

func TestSetChatter_StartsAndStopsBackgroundMessages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		suite := demo.New(1, false)
		stop := runDemo(t, suite, sink)
		defer stop()

		time.Sleep(2 * time.Minute)
		synctest.Wait()

		if got := countIncoming(sink.take()); got != 0 {
			t.Fatalf("chatter off: %d incoming messages, want 0", got)
		}

		suite.SetChatter(true)
		time.Sleep(2 * time.Minute)
		synctest.Wait()

		if got := countIncoming(sink.take()); got < 3 {
			t.Errorf("chatter on: %d incoming messages, want at least one per account", got)
		}
	})
}

func TestInject_DeliversIntoTheOwningAccount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		suite := demo.New(1, false)
		stop := runDemo(t, suite, sink)
		defer stop()

		waitConnected(sink)

		m, err := suite.Inject(t.Context(), "tg:nadia")
		if err != nil || m.SenderName != "Nadia" {
			t.Fatalf("Inject = %+v, %v", m, err)
		}

		if !sink.has("incoming tg:nadia " + m.RemoteID) {
			t.Errorf("events = %v", sink.take())
		}

		if _, err := suite.Inject(t.Context(), "wa:nobody"); !errors.Is(err, demo.ErrUnknownConversation) {
			t.Errorf("Inject(unknown) = %v, want ErrUnknownConversation", err)
		}
	})
}

func TestInject_RefusesWhenStopped(t *testing.T) {
	t.Parallel()

	if _, err := demo.New(1, false).Inject(t.Context(), "wa:mum"); !errors.Is(err, demo.ErrNotRunning) {
		t.Errorf("Inject = %v, want ErrNotRunning", err)
	}
}

// countIncoming counts the incoming message events.
func countIncoming(events []string) int {
	n := 0
	for _, e := range events {
		if strings.HasPrefix(e, "incoming ") {
			n++
		}
	}

	return n
}
