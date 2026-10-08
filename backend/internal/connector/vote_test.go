package connector_test

// Manager.Vote's own test sits apart from manager_test.go's other
// capability tests only to keep that file under this repository's line
// limit; it otherwise belongs there, beside React's and Organizer's own
// asks-the-right-connector tests (see delete_test.go for the same
// reason).

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestVote_AsksConnectorsThatSupportVoting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withVoter{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.Vote(ctx, domain.Conversation{AccountID: "tg"}, "40", []string{"a"}); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(tg.voted, []string{"40 [a]"}) {
			t.Errorf("voted %v, want [40 [a]]", tg.voted)
		}

		if err := m.Vote(ctx, domain.Conversation{AccountID: "plain"}, "40", []string{"a"}); !errors.Is(err, connector.ErrNoVoter) {
			t.Errorf("Vote on a connector without polls = %v, want ErrNoVoter", err)
		}

		if err := m.Vote(ctx, domain.Conversation{AccountID: "nobody"}, "40", []string{"a"}); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("Vote for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}
