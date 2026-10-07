package fake_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestConformance_Lifecycle runs the shared lifecycle check: a fresh fake
// connector must connect, honour cancellation, refuse a second concurrent
// run and leave no goroutine running.
func TestConformance_Lifecycle(t *testing.T) {
	t.Parallel()

	connectortest.CheckLifecycle(t, func(t *testing.T) connector.Connector {
		t.Helper()

		return fake.New().Connectors()[0]
	})
}

// TestConformance_SendProgress runs the shared send-progress check against
// a direct chat's delivery receipts.
func TestConformance_SendProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		wa := suite.Connectors()[0]
		mum := conversation("wa-personal", "wa:mum", domain.KindDirect)
		if err := wa.Send(t.Context(), mum, domain.Message{ID: "progress"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(receiptWait)
		synctest.Wait()

		connectortest.CheckSendProgress(t, sink, "progress")
	})
}

// TestConformance_Incoming runs the shared incoming-fields check against a
// scripted message injected into a conversation.
func TestConformance_Incoming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		m, err := suite.Inject(t.Context(), "wa:mum")
		if err != nil {
			t.Fatal(err)
		}

		connectortest.CheckIncoming(t, m)
	})
}

// TestConformance_Duplicates runs the shared duplicate-delivery check
// against a conversation's seeded history, which the fake connector
// reports again, unchanged, on every reconnect.
func TestConformance_Duplicates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		suite := fake.New()

		stop := runFake(t, suite, sink)
		waitConnected(sink)
		seeded := sink.Messages()["wa:mum"]
		stop()

		stop = runFake(t, suite, sink)
		defer stop()
		waitConnected(sink)
		reseeded := sink.Messages()["wa:mum"]

		if len(seeded) == 0 || len(reseeded) != 2*len(seeded) {
			t.Fatalf("seeded history = %d, reseeded = %d; want the same history reported again", len(seeded), len(reseeded))
		}

		connectortest.CheckDuplicates(t, seeded[0], reseeded[len(seeded)])
	})
}
