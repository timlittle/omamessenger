package connector_test

// Manager.DeleteMessages' own test sits apart from manager_test.go's
// other capability tests only to keep that file under this repository's
// line limit; it otherwise belongs there, beside React's and
// Organizer's own asks-the-right-connector tests.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestDeleteMessages_AsksConnectorsThatDelete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withDeleter{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.DeleteMessages(ctx, domain.Conversation{AccountID: "tg"}, []string{"40"}, true); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(tg.deleted, []string{"[40] true"}) {
			t.Errorf("deleted %v, want [[40] true]", tg.deleted)
		}

		if err := m.DeleteMessages(ctx, domain.Conversation{AccountID: "plain"}, []string{"40"}, true); !errors.Is(err, connector.ErrNoDeleter) {
			t.Errorf("DeleteMessages on a connector that cannot delete = %v, want ErrNoDeleter", err)
		}

		if err := m.DeleteMessages(ctx, domain.Conversation{AccountID: "nobody"}, []string{"40"}, true); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("DeleteMessages for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}
