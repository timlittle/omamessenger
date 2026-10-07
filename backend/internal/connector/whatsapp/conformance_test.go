package whatsapp

// This file runs the shared connector conformance checks against
// WhatsApp. CheckLifecycle runs offline against a fake device, since
// Run's shape (connect, cancel, refuse a second concurrent run, leave no
// goroutine behind) does not depend on WhatsApp's servers. The other
// checks need messages, contacts and conversations, which a later wave
// adds, so they are not run here; the Telegram connector skips the same
// ones for the same reason.
//
// device, answer and the connector's unexported fields have no public
// way to be driven without a live pairing, so this test constructs
// Connector directly rather than through its exported API.

import (
	"context"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestConformance_Lifecycle runs the shared lifecycle check against an
// account that is already paired, so Run connects straight away.
func TestConformance_Lifecycle(t *testing.T) {
	t.Parallel()

	connectortest.CheckLifecycle(t, func(t *testing.T) connector.Connector {
		t.Helper()

		dev := newFakeDevice()
		dev.paired = true
		media := newTestMediaStore(t)

		return &Connector{
			account:   domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp},
			answers:   make(chan answer, 1),
			open:      func(context.Context) (device, error) { return dev, nil },
			openMedia: func(context.Context) (*mediaStore, error) { return media, nil },
			organize:  map[string]organizeState{},
			names:     map[string]string{},
			reactions: map[string]map[string]string{},
		}
	})
}
