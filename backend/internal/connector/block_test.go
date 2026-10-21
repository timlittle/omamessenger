package connector_test

// Manager.SetBlocked's and Manager.ReportSpam's own tests sit apart from
// manager_test.go's other capability tests only to keep that file under
// this repository's line limit; they otherwise belong there, beside
// Deleter's and Editor's own asks-the-right-connector tests.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSetBlocked_AsksConnectorsThatSupportBlocking(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withBlocker{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})
		stopManager(t, cancel, m)

		if err := m.SetBlocked(ctx, domain.Conversation{AccountID: "tg"}, true); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(tg.blocked, []bool{true}) {
			t.Errorf("blocked %v, want [true]", tg.blocked)
		}

		if err := m.SetBlocked(ctx, domain.Conversation{AccountID: "plain"}, true); !errors.Is(err, connector.ErrNoBlocker) {
			t.Errorf("SetBlocked on a connector without blocking = %v, want ErrNoBlocker", err)
		}

		if err := m.SetBlocked(ctx, domain.Conversation{AccountID: "nobody"}, true); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("SetBlocked for an unknown account = %v", err)
		}
	})
}

func TestReportSpam_AsksConnectorsThatSupportReportingSpam(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withSpamReporter{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})
		stopManager(t, cancel, m)

		if err := m.ReportSpam(ctx, domain.Conversation{AccountID: "tg"}); err != nil {
			t.Fatal(err)
		}
		if tg.reported != 1 {
			t.Errorf("reported = %d, want 1", tg.reported)
		}

		if err := m.ReportSpam(ctx, domain.Conversation{AccountID: "plain"}); err != nil {
			t.Errorf("ReportSpam on a connector without reporting = %v, want nothing", err)
		}

		if err := m.ReportSpam(ctx, domain.Conversation{AccountID: "nobody"}); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("ReportSpam for an unknown account = %v", err)
		}
	})
}
