package demo_test

import (
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/connector/demo"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSeed_HistoryHasScriptedCountsAndUnread(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		stop := runDemo(t, demo.New(1, false), sink)
		synctest.Wait()
		stop()

		want := map[string]struct{ count, unread int }{
			"wa:mum": {14, 0}, "wa:climbing-crew": {30, 3}, "wa:alex-chen": {6, 1},
			"wa:sam-spotty": {4, 0}, "wa:flat-4b": {12, 5}, "wa:dentist": {2, 0},
			"tg:nadia": {10, 2}, "tg:omarchy-users": {150, 12}, "tg:saved-messages": {5, 0},
			"tg:platform-team": {20, 4}, "tg:jordan-manager": {8, 0},
		}

		if len(sink.history) != len(want) {
			t.Fatalf("seeded %d conversations, want %d", len(sink.history), len(want))
		}

		for remoteID, w := range want {
			messages := sink.history[remoteID]
			if len(messages) != w.count || unread(messages) != w.unread {
				t.Errorf("%s: %d messages, %d unread; want %d, %d",
					remoteID, len(messages), unread(messages), w.count, w.unread)
			}
		}
	})
}

func TestSeed_HistoryIsOrderedAndAttributed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		stop := runDemo(t, demo.New(1, false), sink)
		synctest.Wait()
		stop()

		for remoteID, messages := range sink.history {
			for i, m := range messages {
				if i > 0 && m.Created <= messages[i-1].Created {
					t.Errorf("%s: message %d is not newer than the one before", remoteID, i)
				}

				if m.Outgoing != (m.SenderName == "You") {
					t.Errorf("%s: message %d outgoing=%t sender=%q", remoteID, i, m.Outgoing, m.SenderName)
				}
			}
		}

		saved := sink.history["tg:saved-messages"]
		for _, m := range saved {
			if !m.Outgoing {
				t.Errorf("saved message %q is incoming", m.Text)
			}
		}

		jordan := sink.history["tg:jordan-manager"]
		if last := jordan[len(jordan)-1]; !last.Outgoing || last.Status != domain.StatusRead {
			t.Errorf("Jordan's newest message = %+v, want outgoing and read", last)
		}
	})
}

// unread counts the messages still marked received.
func unread(messages []domain.Message) int {
	n := 0
	for _, m := range messages {
		if m.Status == domain.StatusReceived {
			n++
		}
	}

	return n
}
