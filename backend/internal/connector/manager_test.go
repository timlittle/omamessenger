package connector_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestNewManager_RejectsInvalidConnectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connectors []connector.Connector
	}{
		{"nil connector", []connector.Connector{nil}},
		{"empty account id", []connector.Connector{&fakeConnector{}}},
		{"duplicate account", []connector.Connector{&fakeConnector{id: "a"}, &fakeConnector{id: "a"}}},
	}

	for _, tt := range tests {
		if _, err := connector.NewManager(tt.connectors...); err == nil {
			t.Errorf("%s: NewManager succeeded", tt.name)
		}
	}
}

func TestStart_RecordsAccountsBeforeRunning(t *testing.T) {
	t.Parallel()

	m, err := connector.NewManager(&fakeConnector{id: "a"}, &fakeConnector{id: "b"})
	if err != nil {
		t.Fatal(err)
	}

	accounts := &accountList{}
	ctx, cancel := context.WithCancel(t.Context())
	if err := m.Start(ctx, accounts, &statusSink{}); err != nil {
		t.Fatal(err)
	}

	cancel()
	m.Wait()

	if !slices.Equal(accounts.ids, []string{"a", "b"}) {
		t.Errorf("recorded accounts = %v, want [a b]", accounts.ids)
	}
}

func TestStart_FailsWhenAccountCannotBeRecorded(t *testing.T) {
	t.Parallel()

	m, err := connector.NewManager(&fakeConnector{id: "a"})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Start(t.Context(), &accountList{err: errBroken}, &statusSink{}); !errors.Is(err, errBroken) {
		t.Fatalf("Start = %v, want errBroken", err)
	}
}

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

		if got := sink.recorded()[0]; got != "a:error:broken" {
			t.Errorf("first status = %q, want a:error:broken", got)
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

		if got := sink.recorded()[0]; got != "a:error:connector stopped unexpectedly" {
			t.Errorf("first status = %q", got)
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

		if got := sink.recorded(); len(got) != 0 {
			t.Errorf("statuses = %v, want none", got)
		}
	})
}

func TestSend_RoutesByAccount(t *testing.T) {
	t.Parallel()

	a, b := &fakeConnector{id: "a"}, &fakeConnector{id: "b"}
	m, err := connector.NewManager(a, b)
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	if err := m.Send(ctx, domain.Conversation{AccountID: "b"}, domain.Message{ID: "m1"}); err != nil {
		t.Fatal(err)
	}

	if len(a.sent) != 0 || !slices.Equal(b.sent, []string{"m1"}) {
		t.Errorf("sent a=%v b=%v, want only b=[m1]", a.sent, b.sent)
	}

	if err := m.MarkRead(ctx, domain.Conversation{AccountID: "a"}); err != nil {
		t.Errorf("MarkRead = %v", err)
	}

	for _, err := range []error{
		m.Send(ctx, domain.Conversation{AccountID: "x"}, domain.Message{}),
		m.MarkRead(ctx, domain.Conversation{AccountID: "x"}),
	} {
		if !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("unknown account = %v, want ErrNoConnector", err)
		}
	}
}
