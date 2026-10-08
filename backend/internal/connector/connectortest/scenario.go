package connectortest

// Scenario tests reproduce the class of bug a unit test with one tidy
// input never sees: a helper that restarts mid-session, a service that
// delivers the same events in a different order than the happy path
// assumed, and a redelivery after a reconnect. Restart, Reorder and
// DuplicateDelivery below run the same checks against any connector
// that implements Driver.
//
// To adopt them, a connector's test package writes a Driver: Connector
// returns a fresh connector wired to whatever fake backend and on-disk
// state an earlier Connector call left behind, Run starts it against a
// sink, and Deliver turns one Event into that fake backend's own calls.
// See whatsapp's and telegram's scenario_test.go for worked examples.
// Restart additionally takes a verify func, since checking that
// MarkRead, a pin or a media fetch survived a restart needs the
// connector's own capability interfaces, which this package cannot
// name without depending on every connector that implements them.

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// EventKind names which of Event's fields apply.
type EventKind int

// The kinds of update Restart, Reorder and DuplicateDelivery can
// script, in the shape every connector's Sink reports them.
const (
	EventConversation EventKind = iota // a conversation becomes known
	EventMessage                       // a message, live or history
	EventOrganize                      // a pin or archive change
	EventRead                          // the service's own read state
	EventDelete                        // messages removed from the service
)

// Event is one scripted service update, in a shape common to every
// connector; a Driver's Deliver translates it into whatever its own
// fake backend expects.
type Event struct {
	// Kind says which of the fields below apply.
	Kind EventKind

	// ConversationRemoteID names the conversation every kind acts on.
	ConversationRemoteID string

	// Title and ConversationKind apply to EventConversation: the
	// conversation's display name and domain.KindDirect or
	// domain.KindGroup.
	Title            string
	ConversationKind string

	// MessageRemoteID, SenderID, SenderName, Text and Live apply to
	// EventMessage: Live true delivers it as a live message, false as
	// an earlier one from history.
	MessageRemoteID string
	SenderID        string
	SenderName      string
	Text            string
	Live            bool

	// Pinned and Archived apply to EventOrganize; a nil field leaves
	// that one alone, the way a connector's own live pin-only or
	// archive-only update does.
	Pinned, Archived *bool

	// Unread applies to EventRead: the service's own unread count.
	Unread int

	// DeleteRemoteIDs applies to EventDelete: the message remote ids
	// removed from ConversationRemoteID.
	DeleteRemoteIDs []string
}

// Driver is the one seam a connector's test implements so Restart,
// Reorder and DuplicateDelivery can run that connector against its own
// scripted fake without this package knowing WhatsApp's or Telegram's
// wire types. It carries more than the usual two methods because the
// scenarios need three different things from a connector under test: a
// fresh instance over whatever state an earlier instance left behind, a
// way to start that instance running against a sink, and a way to feed
// one scripted event to the fake backend it is currently listening to.
type Driver interface {
	// Connector returns a new, not yet run connector wired to this
	// driver's fake backend and whatever state an earlier Connector
	// call left on disk, exactly as a process restart would see it.
	Connector(t *testing.T) connector.Connector

	// Run starts c against sink, inside the caller's synctest bubble,
	// and returns a stop func that ends the run the way Restart needs
	// to before the next Connector call.
	Run(t *testing.T, c connector.Connector, sink connector.Sink) (stop func())

	// Deliver feeds one scripted event to the fake backend the
	// currently running connector is listening to.
	Deliver(t *testing.T, event Event)

	// Backend returns the fake service instance Deliver and Run acted
	// on most recently, for a connector's own test to inspect what
	// reached it beyond what any Sink update reports, such as a
	// markRead call or a media download. Its concrete type is always
	// the connector's own fake, never this package's.
	Backend() any
}

// CheckRestart runs one connector instance through seed, stops it, and
// hands a second instance over the same on-disk state — exactly what a
// helper restart or a re-pair leaves behind — to verify. The connector's
// test checks there that MarkRead, a pin, a media fetch or a reply
// still reach the fake service using nothing kept only in the first
// instance's memory.
func CheckRestart(t *testing.T, newDriver func(t *testing.T) Driver, seed []Event, verify func(t *testing.T, d Driver, c connector.Connector, sink *Sink)) {
	t.Helper()

	synctest.Test(t, func(t *testing.T) {
		d := newDriver(t)

		c1 := d.Connector(t)
		sink1 := &Sink{}
		stop1 := d.Run(t, c1, sink1)
		for _, e := range seed {
			d.Deliver(t, e)
			synctest.Wait()
		}
		stop1()
		synctest.Wait()

		c2 := d.Connector(t)
		sink2 := &Sink{}
		stop2 := d.Run(t, c2, sink2)
		defer stop2()

		verify(t, d, c2, sink2)
	})
}

// CheckReorder delivers events to a fresh connector in their given
// order, then again in each of Orderings' adversarial reorderings, each
// time against a brand new driver, and asserts the sink's final state
// agrees every time: a reorder must never leave a different result, such
// as a pin applied before its conversation existed being lost, or a
// later snapshot reverting a live update it should not.
func CheckReorder(t *testing.T, newDriver func(t *testing.T) Driver, events []Event) {
	t.Helper()

	want := runScenario(t, newDriver(t), events)

	for i, ordering := range Orderings(events) {
		got := runScenario(t, newDriver(t), ordering)
		if diff := diffSnapshots(want, got); diff != "" {
			t.Errorf("ordering %d produced a different final state: %s", i, diff)
		}
	}
}

// CheckDuplicateDelivery delivers events once, then again with every
// event delivered twice in a row, and asserts the final state is the
// same either way: a redelivered message, edit or delete, the way a
// reconnect's resync can produce, must never duplicate or revert
// anything.
func CheckDuplicateDelivery(t *testing.T, newDriver func(t *testing.T) Driver, events []Event) {
	t.Helper()

	once := runScenario(t, newDriver(t), events)

	doubled := make([]Event, 0, len(events)*2)
	for _, e := range events {
		doubled = append(doubled, e, e)
	}
	twice := runScenario(t, newDriver(t), doubled)

	if diff := diffSnapshots(once, twice); diff != "" {
		t.Errorf("delivering every event twice changed the final state: %s", diff)
	}
}

// runScenario starts a fresh connector from d, delivers each event in
// order, stops it, and returns the sink's final state, inside its own
// synctest bubble so any connector's own connect delay advances
// instantly.
func runScenario(t *testing.T, d Driver, events []Event) map[string]ConversationSnapshot {
	t.Helper()

	var snapshot map[string]ConversationSnapshot
	synctest.Test(t, func(t *testing.T) {
		c := d.Connector(t)
		sink := &Sink{}
		stop := d.Run(t, c, sink)

		for _, e := range events {
			d.Deliver(t, e)
			synctest.Wait()
		}

		stop()
		synctest.Wait()
		snapshot = sink.Snapshot()
	})

	return snapshot
}

// Orderings returns a handful of adversarial deliveries of the same
// events, picked for the bugs this suite guards against rather than
// every possible permutation, which is unreadable past a handful of
// events: the events as given, fully reversed, and one ordering per
// event that promotes it to arrive first while leaving every other
// event's relative order alone, the shape of a pin arriving before the
// conversation it belongs to, or a history snapshot arriving after a
// live update it must not revert.
func Orderings(events []Event) [][]Event {
	if len(events) < 2 {
		return [][]Event{slices.Clone(events)}
	}

	orderings := [][]Event{slices.Clone(events), reversed(events)}
	for i := range events {
		orderings = append(orderings, promote(events, i))
	}

	return orderings
}

// reversed returns events in the opposite order.
func reversed(events []Event) []Event {
	out := make([]Event, len(events))
	for i, e := range events {
		out[len(events)-1-i] = e
	}

	return out
}

// promote returns events with the one at i moved to the front, keeping
// every other event's relative order.
func promote(events []Event, i int) []Event {
	out := make([]Event, 0, len(events))
	out = append(out, events[i])
	for j, e := range events {
		if j != i {
			out = append(out, e)
		}
	}

	return out
}

// diffSnapshots describes how two sinks' final states differ, or ""
// when they agree.
func diffSnapshots(want, got map[string]ConversationSnapshot) string {
	var b strings.Builder

	for remote, w := range want {
		g, ok := got[remote]
		if !ok {
			fmt.Fprintf(&b, "conversation %s missing, want %+v; ", remote, w)
			continue
		}
		if !reflect.DeepEqual(w, g) {
			fmt.Fprintf(&b, "conversation %s = %+v, want %+v; ", remote, g, w)
		}
	}

	for remote, g := range got {
		if _, ok := want[remote]; !ok {
			fmt.Fprintf(&b, "conversation %s unexpected: %+v; ", remote, g)
		}
	}

	return b.String()
}
