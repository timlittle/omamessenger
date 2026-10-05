package connector

import (
	"sync"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// callSink records which Sink methods ran, in order.
type callSink struct {
	mu    sync.Mutex
	calls []string
}

func (s *callSink) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}
func (s *callSink) AccountStatus(_, status, _ string)       { s.record("status:" + status) }
func (s *callSink) Contact(domain.Contact)                  { s.record("contact") }
func (s *callSink) Conversation(domain.Conversation)        { s.record("conversation") }
func (s *callSink) Incoming(string, string, domain.Message) { s.record("incoming") }
func (s *callSink) OutgoingStatus(string, string, string)   { s.record("outgoing") }
func (s *callSink) Typing(string, string, string, bool)     { s.record("typing") }
func (s *callSink) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// historySink is a callSink that also accepts backfill.
type historySink struct{ callSink }

func (s *historySink) History(string, string, domain.Message) { s.record("history") }

func TestTrackedSinkRoutesHistory(t *testing.T) {
	plain := &callSink{}
	trackedSink{Sink: plain}.History("wa", "r", domain.Message{})
	if got := plain.snapshot(); len(got) != 1 || got[0] != "incoming" {
		t.Errorf("history to a plain sink = %v, want incoming", got)
	}
	backfill := &historySink{}
	trackedSink{Sink: backfill}.History("wa", "r", domain.Message{})
	if got := backfill.snapshot(); len(got) != 1 || got[0] != "history" {
		t.Errorf("history to a HistorySink = %v, want history", got)
	}
}

// TestTrackedSinkCountsOnlyConnectedTime checks the accounting that decides
// whether a long-lived connection resets the restart backoff: time spent
// disconnected, and status for other accounts, must not count.
func TestTrackedSinkCountsOnlyConnectedTime(t *testing.T) {
	clock := clocktest.New(time.Unix(0, 0))
	state := &connectionState{}
	sink := &callSink{}
	tracked := trackedSink{Sink: sink, Clock: clock, AccountID: "wa", State: state}

	tracked.AccountStatus("wa", domain.AccountConnected, "")
	clock.Advance(3 * time.Minute)
	tracked.AccountStatus("wa", domain.AccountOffline, "")
	clock.Advance(time.Hour)
	tracked.AccountStatus("other", domain.AccountConnected, "")
	tracked.AccountStatus("wa", domain.AccountConnected, "")
	tracked.AccountStatus("wa", domain.AccountConnected, "")
	clock.Advance(2 * time.Minute)

	if got := state.connectedDuration(clock.Now()); got != 5*time.Minute {
		t.Errorf("connected duration = %v, want 5m", got)
	}
	if got := len(sink.snapshot()); got != 5 {
		t.Errorf("forwarded %d status calls, want all 5", got)
	}
}

func TestRealClock(t *testing.T) {
	clock := RealClock{}
	if now := clock.Now(); time.Since(now) > time.Minute || time.Until(now) > time.Minute {
		t.Fatalf("Now() = %v", now)
	}
	fired := make(chan struct{})
	clock.AfterFunc(time.Millisecond, func() { close(fired) })
	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("AfterFunc did not fire")
	}
	stop := clock.AfterFunc(time.Hour, func() { t.Error("stopped timer fired") })
	if !stop() {
		t.Error("stop() on a pending timer = false")
	}
}
