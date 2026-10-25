package app_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errBoom is a distinct error for a test that only needs some error, to
// drive a fake dispatcher's own refusal to send.
var errBoom = errors.New("boom")

// runScheduledMessages starts f.commands.RunScheduledMessages in its
// own goroutine, returning a function that cancels it and waits for it
// to stop, so every test leaves no goroutine running past its own end.
func runScheduledMessages(t *testing.T, ctx context.Context, cancel context.CancelFunc, f *fixture) func() {
	t.Helper()

	var wg sync.WaitGroup
	wg.Go(func() { f.commands.RunScheduledMessages(ctx) })

	return func() {
		cancel()
		wg.Wait()
	}
}

func TestScheduleMessage_RejectsEmptyText(t *testing.T) {
	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Climbing Crew", domain.KindDirect)

	if _, err := f.commands.ScheduleMessage(t.Context(), chat.ID, "  ", time.Now().Add(time.Hour).UnixMilli()); err == nil {
		t.Fatal("expected an error for empty text")
	}
}

func TestScheduleMessage_RejectsATimeNotInTheFuture(t *testing.T) {
	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Climbing Crew", domain.KindDirect)

	if _, err := f.commands.ScheduleMessage(t.Context(), chat.ID, "see you soon", time.Now().Add(-time.Minute).UnixMilli()); err == nil {
		t.Fatal("expected an error for a time in the past")
	}
}

func TestListScheduled_FiltersByConversationWhenGiven(t *testing.T) {
	f := newFixture(t, false)
	a := f.conversation(t, "a", "A", domain.KindDirect)
	b := f.conversation(t, "b", "B", domain.KindDirect)

	due := time.Now().Add(time.Hour).UnixMilli()
	if _, err := f.commands.ScheduleMessage(t.Context(), a.ID, "to a", due); err != nil {
		t.Fatal(err)
	}
	if _, err := f.commands.ScheduleMessage(t.Context(), b.ID, "to b", due); err != nil {
		t.Fatal(err)
	}

	all, err := f.commands.ListScheduled(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListScheduled(\"\") = %d messages, want 2", len(all))
	}

	onlyA, err := f.commands.ListScheduled(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyA) != 1 || onlyA[0].Text != "to a" {
		t.Errorf("ListScheduled(a) = %+v, want one message, text \"to a\"", onlyA)
	}
}

func TestEditScheduled_ChangesTextAndTime(t *testing.T) {
	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	m, err := f.commands.ScheduleMessage(t.Context(), chat.ID, "draft", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}

	newAt := time.Now().Add(2 * time.Hour).UnixMilli()
	updated, err := f.commands.EditScheduled(t.Context(), m.ID, "final text", newAt)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Text != "final text" || updated.SendAt != newAt {
		t.Errorf("EditScheduled result = %+v, want text=%q sendAt=%d", updated, "final text", newAt)
	}
}

func TestCancelScheduled_RemovesItAndItNeverSends(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

		m, err := f.commands.ScheduleMessage(ctx, chat.ID, "never mind", time.Now().Add(time.Hour).UnixMilli())
		if err != nil {
			t.Fatal(err)
		}

		if err := f.commands.CancelScheduled(ctx, m.ID); err != nil {
			t.Fatal(err)
		}

		remaining, err := f.commands.ListScheduled(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(remaining) != 0 {
			t.Fatalf("ListScheduled after cancel = %+v, want none", remaining)
		}

		stop := runScheduledMessages(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		time.Sleep(2 * time.Hour)
		synctest.Wait()

		if got := f.dispatcher.sentTexts(); len(got) != 0 {
			t.Errorf("sent = %v, want none: the message was cancelled before it came due", got)
		}
	})
}

func TestRunScheduledMessages_SendsAtTheDueTimeAndNotBefore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Climbing Crew", domain.KindDirect)

		due := time.Now().Add(time.Hour)
		if _, err := f.commands.ScheduleMessage(ctx, chat.ID, "see you soon", due.UnixMilli()); err != nil {
			t.Fatal(err)
		}
		f.published.take()

		stop := runScheduledMessages(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		if got := f.dispatcher.sentTexts(); len(got) != 0 {
			t.Fatalf("sent before due: %v", got)
		}

		time.Sleep(time.Hour)
		synctest.Wait()

		if got := f.dispatcher.sentTexts(); !slices.Equal(got, []string{"see you soon"}) {
			t.Errorf("sent = %v, want [\"see you soon\"]", got)
		}

		events := f.published.take()
		if !slices.Contains(events, "scheduledMessage.removed") {
			t.Errorf("published = %v, want it to include scheduledMessage.removed", events)
		}
		if !slices.Contains(events, "message.added") {
			t.Errorf("published = %v, want it to include message.added", events)
		}
	})
}

func TestRunScheduledMessages_SendsAtStartupWhenAlreadyOverdue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Overdue Chat", domain.KindDirect)

		// Simulate a message scheduled before a restart, whose due time
		// has already passed while the helper was not running: the
		// machine could have been off, or the helper merely closed.
		past := domainScheduled(chat.ID, "already due", time.Now().Add(-time.Hour).UnixMilli())
		if _, err := f.store.AddScheduledMessage(t.Context(), past); err != nil {
			t.Fatal(err)
		}

		stop := runScheduledMessages(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		if got := f.dispatcher.sentTexts(); !slices.Equal(got, []string{"already due"}) {
			t.Errorf("sent = %v, want [\"already due\"]", got)
		}
	})
}

func TestRunScheduledMessages_NeverSendsTwiceAcrossARestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Restarted Chat", domain.KindDirect)

		due := time.Now().Add(30 * time.Minute)
		if _, err := f.commands.ScheduleMessage(ctx, chat.ID, "restart test", due.UnixMilli()); err != nil {
			t.Fatal(err)
		}

		// The first helper run stops before the message comes due.
		stop := runScheduledMessages(t, ctx, cancel, f)
		synctest.Wait()
		stop()

		if got := f.dispatcher.sentTexts(); len(got) != 0 {
			t.Fatalf("sent before stopping: %v", got)
		}

		// A second Commands and Ingest over the same database, as
		// main.go builds after an actual process restart, must still
		// send it exactly once, not lose track of it and not duplicate
		// whatever the first run might have already started.
		commands2, _, dispatcher2, _ := appOver(t, f.store)
		ctx2, cancel2 := context.WithCancel(t.Context())

		var wg sync.WaitGroup
		wg.Go(func() { commands2.RunScheduledMessages(ctx2) })
		t.Cleanup(func() { cancel2(); wg.Wait() })
		synctest.Wait()

		time.Sleep(30 * time.Minute)
		synctest.Wait()

		if got := dispatcher2.sentTexts(); !slices.Equal(got, []string{"restart test"}) {
			t.Errorf("sent after restart = %v, want [\"restart test\"] exactly once", got)
		}

		remaining, err := f.store.ScheduledMessages(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(remaining) != 0 {
			t.Errorf("remaining scheduled messages = %+v, want none: it was claimed and sent", remaining)
		}
	})
}

func TestRunScheduledMessages_FailedSendBehavesLikeAnyFailedSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
		f.dispatcher.err = errBoom

		due := time.Now().Add(time.Hour)
		if _, err := f.commands.ScheduleMessage(ctx, chat.ID, "will fail", due.UnixMilli()); err != nil {
			t.Fatal(err)
		}

		stop := runScheduledMessages(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		time.Sleep(time.Hour)
		synctest.Wait()

		messages, _, _, err := f.commands.Messages(ctx, chat.ID, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != 1 || messages[0].Status != domain.StatusFailed {
			t.Fatalf("messages = %+v, want one failed message", messages)
		}
		if messages[0].RetryAt == 0 {
			t.Error("expected a failed scheduled send to be scheduled for automatic retry, like any other failed send")
		}
	})
}

func TestSendScheduledNow_SendsImmediatelyAndRemovesTheQueueEntry(t *testing.T) {
	f := newFixture(t, false)
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	m, err := f.commands.ScheduleMessage(t.Context(), chat.ID, "send now", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}

	sent, err := f.commands.SendScheduledNow(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sent.Text != "send now" || sent.Status == "" {
		t.Errorf("SendScheduledNow result = %+v, want a sent message", sent)
	}

	remaining, err := f.commands.ListScheduled(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining = %+v, want none", remaining)
	}
}

func TestRunScheduledMessages_StopsWhenContextIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())

		stop := runScheduledMessages(t, ctx, cancel, f)
		stop() // returns once RunScheduledMessages has actually stopped; no goroutine leak
	})
}

// domainScheduled builds a domain.ScheduledMessage for tests that need
// to store one directly through the store, bypassing
// Commands.ScheduleMessage's own future-time validation, to simulate a
// message queued before a restart whose time has since passed.
func domainScheduled(conversationID, text string, sendAt int64) domain.ScheduledMessage {
	return domain.ScheduledMessage{ConversationID: conversationID, Text: text, SendAt: sendAt}
}
