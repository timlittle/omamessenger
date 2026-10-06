package fake_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// runFake starts every fake connector inside the current synctest bubble
// and returns a function that stops them and waits for Run to return.
func runFake(t *testing.T, suite *fake.Suite, sink connector.Sink) (stop func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	for _, c := range suite.Connectors() {
		wg.Go(func() {
			if err := c.Run(ctx, sink); err != nil {
				t.Errorf("Run(%s) = %v", c.Account().ID, err)
			}
		})
	}

	return func() {
		cancel()
		wg.Wait()
	}
}

// conversation returns a fake conversation as the app would pass it.
func conversation(accountID, remoteID, kind string) domain.Conversation {
	return domain.Conversation{AccountID: accountID, RemoteID: remoteID, Kind: kind}
}

// waitConnected lets every fake account connect, then forgets the events
// so far. It must run inside a synctest bubble.
func waitConnected(sink *connectortest.Sink) {
	time.Sleep(2 * time.Second)
	synctest.Wait()
	sink.Take()
}
