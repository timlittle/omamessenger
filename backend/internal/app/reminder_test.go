package app_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// runReminders starts f.commands.RunReminders in its own goroutine,
// returning a function that cancels it and waits for it to stop, so
// every test leaves no goroutine running past its own end.
func runReminders(t *testing.T, ctx context.Context, cancel context.CancelFunc, f *fixture) func() {
	t.Helper()

	var wg sync.WaitGroup
	wg.Go(func() { f.commands.RunReminders(ctx) })

	return func() {
		cancel()
		wg.Wait()
	}
}

func TestRunReminders_FiresAtTheDueTimeAndNotNotBefore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Climbing Crew", domain.KindDirect)

		due := time.Now().Add(time.Hour)
		if _, err := f.commands.SetReminder(ctx, chat.ID, due.UnixMilli()); err != nil {
			t.Fatal(err)
		}
		f.published.take()

		stop := runReminders(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		if got := f.notifier.all(); len(got) != 0 {
			t.Fatalf("notified before due: %v", got)
		}

		time.Sleep(time.Hour)
		synctest.Wait()

		want := []string{"Reminder: Climbing Crew: Snoozed conversation"}
		if got := f.notifier.all(); !slices.Equal(got, want) {
			t.Errorf("notified = %v, want %v", got, want)
		}
		if got := f.notifier.conversations(); !slices.Equal(got, []string{chat.ID}) {
			t.Errorf("notification conversation ids = %v, want [%s]", got, chat.ID)
		}

		if got := f.published.take(); !slices.Equal(got, []string{"conversation.updated"}) {
			t.Errorf("published = %v, want one conversation.updated", got)
		}
	})
}

func TestRunReminders_FiresAtStartupWhenAlreadyOverdue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Overdue Chat", domain.KindDirect)

		// Simulate a reminder set before a restart, whose due time has
		// already passed while the helper was not running.
		past := time.Now().Add(-time.Hour)
		if _, err := f.commands.SetReminder(ctx, chat.ID, past.UnixMilli()); err != nil {
			t.Fatal(err)
		}
		f.published.take()

		stop := runReminders(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		if got := f.notifier.conversations(); !slices.Equal(got, []string{chat.ID}) {
			t.Errorf("notification conversation ids = %v, want [%s]", got, chat.ID)
		}
	})
}

func TestRunReminders_ReArmsWhenAnEarlierReminderIsSet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		later := f.conversation(t, "later", "Later Chat", domain.KindDirect)
		sooner := f.conversation(t, "sooner", "Sooner Chat", domain.KindDirect)

		if _, err := f.commands.SetReminder(ctx, later.ID, time.Now().Add(2*time.Hour).UnixMilli()); err != nil {
			t.Fatal(err)
		}

		stop := runReminders(t, ctx, cancel, f)
		defer stop()
		synctest.Wait()

		// A reminder due sooner than the one already armed must wake the
		// scheduler at once, not wait for the first timer it set.
		if _, err := f.commands.SetReminder(ctx, sooner.ID, time.Now().Add(10*time.Minute).UnixMilli()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		time.Sleep(10 * time.Minute)
		synctest.Wait()

		if got := f.notifier.conversations(); !slices.Equal(got, []string{sooner.ID}) {
			t.Errorf("notification conversation ids = %v, want [%s] (not yet the 2h one)", got, sooner.ID)
		}

		time.Sleep(110 * time.Minute)
		synctest.Wait()

		if got := f.notifier.conversations(); !slices.Equal(got, []string{sooner.ID, later.ID}) {
			t.Errorf("notification conversation ids = %v, want [%s %s]", got, sooner.ID, later.ID)
		}
	})
}

func TestRunReminders_ReArmsAfterARestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())
		chat := f.conversation(t, "chat", "Restarted Chat", domain.KindDirect)

		due := time.Now().Add(30 * time.Minute)
		if _, err := f.commands.SetReminder(ctx, chat.ID, due.UnixMilli()); err != nil {
			t.Fatal(err)
		}

		// The first helper run stops without the reminder firing.
		stop := runReminders(t, ctx, cancel, f)
		synctest.Wait()
		stop()

		if got := f.notifier.all(); len(got) != 0 {
			t.Fatalf("notified before stopping: %v", got)
		}

		// A second Commands and Ingest over the same database, as main.go
		// builds after an actual process restart, must re-arm for the
		// reminder it finds already stored rather than losing track of it.
		commands2, _, _, notifier2 := appOver(t, f.store)
		ctx2, cancel2 := context.WithCancel(t.Context())

		var wg sync.WaitGroup
		wg.Go(func() { commands2.RunReminders(ctx2) })
		t.Cleanup(func() { cancel2(); wg.Wait() })
		synctest.Wait()

		time.Sleep(30 * time.Minute)
		synctest.Wait()

		if got := notifier2.conversations(); !slices.Equal(got, []string{chat.ID}) {
			t.Errorf("notification conversation ids after restart = %v, want [%s]", got, chat.ID)
		}
	})
}

func TestRunReminders_StopsWhenContextIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture(t, false)
		ctx, cancel := context.WithCancel(t.Context())

		stop := runReminders(t, ctx, cancel, f)
		stop() // returns once RunReminders has actually stopped; no goroutine leak
	})
}
