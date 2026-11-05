package signal

// This file runs the shared connector conformance checks against
// Signal, offline against a fake device, the same as WhatsApp's own.

import (
	"testing"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
)

// TestConformance runs the one lifecycle suite every connector runs
// against: connect, cancel, refusing a second concurrent run, and
// leaving no goroutine behind.
func TestConformance(t *testing.T) {
	t.Parallel()

	connectortest.CheckLifecycle(t, func(t *testing.T) connector.Connector {
		t.Helper()

		dev := newFakeDevice(uuid.New())
		c := newTestConnector(dev)

		return c
	})
}
