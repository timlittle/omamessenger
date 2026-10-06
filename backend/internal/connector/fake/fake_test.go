package fake_test

import (
	"errors"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
)

func TestNew_CreatesTheDemoAccounts(t *testing.T) {
	t.Parallel()

	var ids []string
	for _, c := range fake.New().Connectors() {
		ids = append(ids, c.Account().ID)
	}

	if want := []string{"wa-personal", "tg-personal", "tg-work"}; !slices.Equal(ids, want) {
		t.Errorf("accounts = %v, want %v", ids, want)
	}
}

func TestInject_DeliversIntoTheOwningAccount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		m, err := suite.Inject(t.Context(), "tg:nadia")
		if err != nil || m.SenderName != "Nadia" {
			t.Fatalf("Inject = %+v, %v", m, err)
		}

		if !sink.has("incoming tg:nadia " + m.RemoteID) {
			t.Errorf("events = %v", sink.take())
		}

		if _, err := suite.Inject(t.Context(), "wa:nobody"); !errors.Is(err, fake.ErrUnknownConversation) {
			t.Errorf("Inject(unknown) = %v, want ErrUnknownConversation", err)
		}
	})
}

func TestInject_RefusesWhenStopped(t *testing.T) {
	t.Parallel()

	if _, err := fake.New().Inject(t.Context(), "wa:mum"); !errors.Is(err, fake.ErrNotRunning) {
		t.Errorf("Inject = %v, want ErrNotRunning", err)
	}
}
