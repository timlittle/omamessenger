package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSetReminder_SnoozesAndClears(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if err := s.SetReminder(ctx, "chat", 1000); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.ReminderAt != 1000 {
		t.Fatalf("Conversation after SetReminder = %+v, %v; want ReminderAt 1000", c, err)
	}

	if err := s.SetReminder(ctx, "chat", 0); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.ReminderAt != 0 {
		t.Fatalf("Conversation after clearing = %+v, %v; want ReminderAt 0", c, err)
	}

	if err := s.SetReminder(ctx, "missing", 1000); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetReminder(missing) = %v, want ErrNotFound", err)
	}
}

// TestSetReminder_ResetsAnyEarlierNotification confirms snoozing a
// conversation again - even to a due time that happens to repeat an
// earlier one - clears whatever MarkReminderNotified last recorded, so
// the fresh reminder is always eligible to fire.
func TestSetReminder_ResetsAnyEarlierNotification(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if err := s.SetReminder(ctx, "chat", 1000); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReminderNotified(ctx, "chat", 1000); err != nil {
		t.Fatal(err)
	}

	if err := s.SetReminder(ctx, "chat", 2000); err != nil {
		t.Fatal(err)
	}

	c, err := s.Conversation(ctx, "chat")
	if err != nil || c.ReminderAt != 2000 || c.ReminderNotifiedAt != 0 {
		t.Fatalf("Conversation after re-snoozing = %+v, %v; want ReminderAt 2000, ReminderNotifiedAt 0", c, err)
	}
}

// TestMarkReminderNotified_RecordsTheDueTimeAlreadyFired confirms the
// due time MarkReminderNotified records survives being read back, and
// that it reports ErrNotFound for a conversation that does not exist,
// the same as SetReminder.
func TestMarkReminderNotified_RecordsTheDueTimeAlreadyFired(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if err := s.SetReminder(ctx, "chat", 1000); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReminderNotified(ctx, "chat", 1000); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.ReminderNotifiedAt != 1000 {
		t.Fatalf("Conversation after MarkReminderNotified = %+v, %v; want ReminderNotifiedAt 1000", c, err)
	}

	if err := s.MarkReminderNotified(ctx, "missing", 1000); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MarkReminderNotified(missing) = %v, want ErrNotFound", err)
	}
}

func TestPendingReminders_ListsOnlyActiveOnesSoonestFirst(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "later", "Later")
	addConversation(t, s, "wa", "sooner", "Sooner")
	addConversation(t, s, "wa", "none", "None")

	if err := s.SetReminder(ctx, "later", 2000); err != nil {
		t.Fatal(err)
	}
	if err := s.SetReminder(ctx, "sooner", 1000); err != nil {
		t.Fatal(err)
	}

	got, err := s.PendingReminders(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, c := range got {
		ids = append(ids, c.ID)
	}

	if want := []string{"sooner", "later"}; !slices.Equal(ids, want) {
		t.Errorf("PendingReminders ids = %v, want %v", ids, want)
	}
}
