package signal

// Signal's own client can read synced pin and archive state but cannot
// write it, so this connector deliberately leaves Organizer unimplemented
// and relies on connector.Manager's local-only fallback.

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestConnector_DoesNotOrganize checks that Connector never satisfies
// connector.Organizer; a regression here would mean pin and archive
// start claiming to sync with a service that cannot store them.
func TestConnector_DoesNotOrganize(t *testing.T) {
	t.Parallel()

	var c connector.Connector = &Connector{}
	if _, ok := c.(connector.Organizer); ok {
		t.Error("Connector satisfies connector.Organizer, want pin and archive to stay local")
	}
}

// accountList records the accounts Manager.Start upserts.
type accountList struct{}

// UpsertAccount satisfies connector.AccountUpserter without recording
// anything; the test only needs Start to succeed.
func (accountList) UpsertAccount(context.Context, domain.Account) error { return nil }

// TestManager_KeepsPinAndArchiveLocalForSignal checks that pinning or
// archiving a Signal conversation through the Manager succeeds without
// error, since Manager falls back to a local-only change for a
// connector that is not an Organizer.
func TestManager_KeepsPinAndArchiveLocalForSignal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice(uuid.New())
		c := newTestConnector(dev)

		m, err := connector.NewManager(c)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		if err := m.Start(ctx, accountList{}, &connectortest.Sink{}); err != nil {
			t.Fatal(err)
		}

		conv := domain.Conversation{AccountID: c.Account().ID}
		if err := m.SetPinned(ctx, conv, true); err != nil {
			t.Errorf("SetPinned() error = %v, want nil local-only change", err)
		}
		if err := m.SetArchived(ctx, conv, true); err != nil {
			t.Errorf("SetArchived() error = %v, want nil local-only change", err)
		}
	})
}
