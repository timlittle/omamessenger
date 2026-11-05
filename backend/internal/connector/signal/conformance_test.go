package signal

// This file runs the shared connector conformance checks against
// Signal. CheckLifecycle runs offline against a fake device, since
// Run's shape (connect, cancel, refuse a second concurrent run, leave
// no goroutine behind) is signalmeow's own event-loop structure, not
// anything Signal's servers decide; an internal test is the only way
// to reach the unexported open field a fake device needs (see
// helpers_test.go), the same reason WhatsApp's own conformance test
// stays in its package rather than *_test.

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
