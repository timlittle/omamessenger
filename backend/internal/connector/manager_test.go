package connector_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
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
	if err := m.Start(ctx, accounts, &connectortest.Sink{}); err != nil {
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

	if err := m.Start(t.Context(), &accountList{err: errBroken}, &connectortest.Sink{}); !errors.Is(err, errBroken) {
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

		if got := sink.Lines()[0]; got != "status a error broken" {
			t.Errorf("first status = %q, want status a error broken", got)
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

		if got := sink.Lines()[0]; got != "status a error connector stopped unexpectedly" {
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

		if got := sink.Lines(); len(got) != 0 {
			t.Errorf("statuses = %v, want none", got)
		}
	})
}

func TestSend_RoutesByAccount(t *testing.T) {
	t.Parallel()

	a, b := &fakeConnector{id: "a"}, &fakeConnector{id: "b"}
	ctx, cancel := context.WithCancel(t.Context())
	m, _ := startManager(t, ctx, a, b)
	defer func() {
		cancel()
		m.Wait()
	}()

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

func TestAdd_RunsAConnectorWhileTheManagerRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		m, _ := startManager(t, ctx)

		started := make(chan struct{})
		added := &fakeConnector{id: "b", run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		}}
		if err := m.Add(ctx, added); err != nil {
			t.Fatal(err)
		}
		<-started

		if err := m.Send(ctx, domain.Conversation{AccountID: "b"}, domain.Message{ID: "m1"}); err != nil {
			t.Errorf("Send to the added account = %v", err)
		}

		if err := m.Add(ctx, &fakeConnector{id: "b"}); !errors.Is(err, connector.ErrDuplicateAccount) {
			t.Errorf("adding the same account again = %v, want ErrDuplicateAccount", err)
		}

		cancel()
		m.Wait()
	})
}

func TestRemove_StopsOnlyThatConnector(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		stopped := make(chan string, 2)
		connectorFor := func(id string) *fakeConnector {
			return &fakeConnector{id: id, run: func(ctx context.Context) error {
				<-ctx.Done()
				stopped <- id
				return nil
			}}
		}
		m, _ := startManager(t, ctx, connectorFor("a"), connectorFor("b"))
		synctest.Wait()

		if err := m.Remove(ctx, "a"); err != nil {
			t.Fatal(err)
		}

		if got := <-stopped; got != "a" {
			t.Errorf("stopped %q, want a", got)
		}

		if err := m.Send(ctx, domain.Conversation{AccountID: "a"}, domain.Message{}); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("Send to a removed account = %v, want ErrNoConnector", err)
		}

		if err := m.Remove(ctx, "a"); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("removing it twice = %v, want ErrNoConnector", err)
		}

		cancel()
		m.Wait()
	})
}

func TestRemove_LogsOutAConnectorThatSupportsItBeforeStopping(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withLogout{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg)
		synctest.Wait()

		if err := m.Remove(ctx, "tg"); err != nil {
			t.Fatal(err)
		}

		if !tg.loggedOut {
			t.Error("Remove did not call Logout on a connector that supports it")
		}

		cancel()
		m.Wait()
	})
}

func TestRemove_IgnoresALogoutFailureAndStillStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withLogout{fakeConnector: fakeConnector{id: "tg"}, logoutErr: errors.New("offline")}
		m, _ := startManager(t, ctx, tg)
		synctest.Wait()

		if err := m.Remove(ctx, "tg"); err != nil {
			t.Fatalf("Remove = %v, want nil even when Logout fails", err)
		}

		cancel()
		m.Wait()
	})
}

func TestSubmitAuth_ReachesConnectorsThatSignIn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &signingIn{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.SubmitAuth(ctx, "tg", "code", "12345"); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(tg.answers, []string{"code=12345"}) {
			t.Errorf("answers = %v", tg.answers)
		}

		if err := m.SubmitAuth(ctx, "plain", "code", "1"); !errors.Is(err, connector.ErrNoAuthentication) {
			t.Errorf("SubmitAuth to a connector that does not sign in = %v", err)
		}

		if err := m.SubmitAuth(ctx, "nobody", "code", "1"); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("SubmitAuth to an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}

func TestLoadOlder_AsksConnectorsThatKeepHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withHistory{fakeConnector: fakeConnector{id: "tg"}, count: 3}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if n, err := m.LoadOlder(ctx, domain.Conversation{AccountID: "tg"}, "40", 30); err != nil || n != 3 {
			t.Errorf("LoadOlder = %d, %v; want 3", n, err)
		}

		if !slices.Equal(tg.from, []string{"40"}) {
			t.Errorf("loaded from %v, want [40]", tg.from)
		}

		if n, err := m.LoadOlder(ctx, domain.Conversation{AccountID: "plain"}, "40", 30); err != nil || n != 0 {
			t.Errorf("LoadOlder from a connector without history = %d, %v; want nothing", n, err)
		}

		if _, err := m.LoadOlder(ctx, domain.Conversation{AccountID: "nobody"}, "40", 30); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("LoadOlder for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}

func TestRefreshMessages_AsksConnectorsThatKeepHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withRefresh{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.RefreshMessages(ctx, domain.Conversation{AccountID: "tg"}, []string{"1", "2"}); err != nil {
			t.Fatal(err)
		}

		if want := [][]string{{"1", "2"}}; !slices.EqualFunc(tg.asked, want, slices.Equal) {
			t.Errorf("asked %v, want %v", tg.asked, want)
		}

		if err := m.RefreshMessages(ctx, domain.Conversation{AccountID: "plain"}, []string{"1"}); err != nil {
			t.Errorf("RefreshMessages on a connector without it = %v, want nothing", err)
		}

		if err := m.RefreshMessages(ctx, domain.Conversation{AccountID: "nobody"}, []string{"1"}); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("RefreshMessages for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}

func TestSetPinned_AsksConnectorsThatOrganize(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withOrganizer{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.SetPinned(ctx, domain.Conversation{AccountID: "tg"}, true); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(tg.pinned, []bool{true}) {
			t.Errorf("pinned %v, want [true]", tg.pinned)
		}

		if err := m.SetPinned(ctx, domain.Conversation{AccountID: "plain"}, true); err != nil {
			t.Errorf("SetPinned on a connector that does not organize = %v, want nothing", err)
		}

		if err := m.SetPinned(ctx, domain.Conversation{AccountID: "nobody"}, true); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("SetPinned for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}

func TestSetArchived_AsksConnectorsThatOrganize(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withOrganizer{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.SetArchived(ctx, domain.Conversation{AccountID: "tg"}, true); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(tg.archived, []bool{true}) {
			t.Errorf("archived %v, want [true]", tg.archived)
		}

		if err := m.SetArchived(ctx, domain.Conversation{AccountID: "plain"}, true); err != nil {
			t.Errorf("SetArchived on a connector that does not organize = %v, want nothing", err)
		}

		if err := m.SetArchived(ctx, domain.Conversation{AccountID: "nobody"}, true); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("SetArchived for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}

func TestReact_AsksConnectorsThatSupportReactions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withReactor{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.React(ctx, domain.Conversation{AccountID: "tg"}, "40", "👍"); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(tg.reacted, []string{"40 👍"}) {
			t.Errorf("reacted %v, want [40 👍]", tg.reacted)
		}

		if err := m.React(ctx, domain.Conversation{AccountID: "plain"}, "40", "👍"); !errors.Is(err, connector.ErrNoReactions) {
			t.Errorf("React on a connector without reactions = %v, want ErrNoReactions", err)
		}

		if err := m.React(ctx, domain.Conversation{AccountID: "nobody"}, "40", "👍"); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("React for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}

func TestFetchMedia_AsksConnectorsThatDownloadMedia(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		tg := &withMedia{fakeConnector: fakeConnector{id: "tg"}}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		if err := m.FetchMedia(ctx, domain.Conversation{AccountID: "tg"}, "40", "/tmp/x.part"); err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(tg.fetched, []string{"40 /tmp/x.part"}) {
			t.Errorf("fetched %v", tg.fetched)
		}

		if err := m.FetchMedia(ctx, domain.Conversation{AccountID: "plain"}, "40", "/tmp/x"); !errors.Is(err, connector.ErrNoMedia) {
			t.Errorf("FetchMedia from a connector without media = %v, want ErrNoMedia", err)
		}

		if err := m.FetchMedia(ctx, domain.Conversation{AccountID: "nobody"}, "40", "/tmp/x"); !errors.Is(err, connector.ErrNoConnector) {
			t.Errorf("FetchMedia for an unknown account = %v", err)
		}

		cancel()
		m.Wait()
	})
}

func TestAdd_GivesUpWhenTheManagerIsNotRunning(t *testing.T) {
	t.Parallel()

	m, err := connector.NewManager()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := m.Add(ctx, &fakeConnector{id: "a"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Add before Start = %v, want context.Canceled", err)
	}

	if err := m.Add(ctx, nil); err == nil {
		t.Error("Add(nil) succeeded")
	}
}
