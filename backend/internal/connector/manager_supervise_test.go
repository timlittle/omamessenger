package connector_test

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

func TestSupervise_RestartsWithGrowingDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var starts []time.Duration
		begin := time.Now()
		failing := &fakeConnector{id: "a", run: func(context.Context) error {
			starts = append(starts, time.Since(begin))
			return errBroken
		}}

		ctx, cancel := context.WithCancel(t.Context())
		m, sink := startManager(t, ctx, failing)

		time.Sleep(90 * time.Second)
		cancel()
		m.Wait()

		want := []time.Duration{0, 1 * time.Second, 3 * time.Second, 8 * time.Second, 23 * time.Second, 83 * time.Second}
		if !slices.Equal(starts, want) {
			t.Errorf("starts = %v, want %v", starts, want)
		}

		// The connector's own error text never reaches the account's
		// status: it can carry file paths, account ids or a service's own
		// words, so the UI only ever sees a fixed sentence.
		if got := sink.Lines()[0]; got != "status a error "+connector.ConnectionFailedDetail {
			t.Errorf("first status = %q, want the fixed connection-failed detail", got)
		}
	})
}

// TestSupervise_RestartsAConnectorThatPanics confirms a panic inside a
// connector's own run loop, such as one parsing a malformed message,
// becomes an ordinary failure the supervisor restarts with its usual
// delay, rather than crashing the whole helper and every other account
// with it.
func TestSupervise_RestartsAConnectorThatPanics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runs := 0
		panicking := &fakeConnector{id: "a", run: func(ctx context.Context) error {
			runs++
			if runs == 1 {
				panic("malformed update")
			}
			<-ctx.Done()
			return nil
		}}

		ctx, cancel := context.WithCancel(t.Context())
		m, sink := startManager(t, ctx, panicking)

		time.Sleep(5 * time.Second)
		cancel()
		m.Wait()

		if runs != 2 {
			t.Errorf("runs = %d, want 2: restarted once after the panic", runs)
		}
		if got := sink.Lines()[0]; got != "status a error connector failed unexpectedly" {
			t.Errorf("first status = %q, want status a error connector failed unexpectedly", got)
		}
	})
}

func TestSupervise_ResetsDelayAfterStableRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var starts []time.Duration
		begin := time.Now()
		runs := 0
		flaky := &fakeConnector{id: "a", run: func(context.Context) error {
			starts = append(starts, time.Since(begin))
			runs++
			if runs == 3 {
				time.Sleep(10 * time.Minute) // a long, healthy connection
			}
			return nil
		}}

		ctx, cancel := context.WithCancel(t.Context())
		m, sink := startManager(t, ctx, flaky)

		time.Sleep(13*time.Minute + 30*time.Second)
		cancel()
		m.Wait()

		// After the stable third run, the delay starts again at 1s.
		want := []time.Duration{0, time.Second, 3 * time.Second, 10*time.Minute + 4*time.Second, 10*time.Minute + 6*time.Second}
		if !slices.Equal(starts[:5], want) {
			t.Errorf("starts = %v, want prefix %v", starts, want)
		}

		if got := sink.Lines()[0]; got != "status a error "+connector.ConnectionFailedDetail {
			t.Errorf("first status = %q, want the fixed connection-failed detail", got)
		}
	})
}

func TestSupervise_StopsWithoutErrorOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		m, sink := startManager(t, ctx, &fakeConnector{id: "a"})

		time.Sleep(time.Hour)
		cancel()
		m.Wait()

		if got := sink.Lines(); len(got) != 0 {
			t.Errorf("statuses = %v, want none", got)
		}
	})
}
