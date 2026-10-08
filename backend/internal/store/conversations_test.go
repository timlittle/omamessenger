package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

func TestEnsureConversation_CreatesThenFollowsRemote(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	first := domain.Conversation{ID: "chat", AccountID: "wa", RemoteID: "remote", Title: "Old", Members: 2}
	created, isNew, err := s.EnsureConversation(ctx, first)
	if err != nil || !isNew || created.Kind != domain.KindDirect || created.Service != domain.ServiceWhatsApp {
		t.Fatalf("first EnsureConversation = %+v, %t, %v", created, isNew, err)
	}

	second := domain.Conversation{AccountID: "wa", RemoteID: "remote", Title: "New", Kind: domain.KindGroup, Members: 12}
	updated, isNew, err := s.EnsureConversation(ctx, second)
	if err != nil || isNew || updated.ID != "chat" || updated.Title != "New" ||
		updated.Kind != domain.KindGroup || updated.Members != 12 {
		t.Fatalf("second EnsureConversation = %+v, %t, %v", updated, isNew, err)
	}
}

func TestEnsureConversation_RejectsMissingFields(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	for _, c := range []domain.Conversation{
		{RemoteID: "remote", Title: "Title"},
		{AccountID: "wa", Title: "Title"},
		{AccountID: "wa", RemoteID: "remote"},
	} {
		if _, _, err := s.EnsureConversation(t.Context(), c); !errors.Is(err, store.ErrInvalidConversation) {
			t.Errorf("EnsureConversation(%+v) = %v, want ErrInvalidConversation", c, err)
		}
	}
}

func TestConversation_NotFound(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	if _, err := s.Conversation(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Conversation(missing) = %v, want ErrNotFound", err)
	}

	if _, err := s.ConversationByRemote(t.Context(), "wa", "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConversationByRemote(missing) = %v, want ErrNotFound", err)
	}
}

func TestMarkRead_ReportsChange(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "in", ConversationID: "chat", Text: "one", Created: 1})

	if changed, err := s.MarkRead(ctx, "chat"); err != nil || !changed {
		t.Fatalf("first MarkRead = %t, %v; want true", changed, err)
	}

	if changed, err := s.MarkRead(ctx, "chat"); err != nil || changed {
		t.Fatalf("second MarkRead = %t, %v; want false", changed, err)
	}

	if _, err := s.MarkRead(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MarkRead(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetUnread_TakesTheServicesCount(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "in", ConversationID: "chat", Text: "one", Created: 1})

	if changed, err := s.SetUnread(ctx, "chat", 3); err != nil || !changed {
		t.Fatalf("SetUnread(3) = %t, %v; want a change", changed, err)
	}

	if c, _ := s.Conversation(ctx, "chat"); c.Unread != 3 {
		t.Errorf("unread = %d, want 3", c.Unread)
	}

	if changed, err := s.SetUnread(ctx, "chat", 3); err != nil || changed {
		t.Errorf("SetUnread(3) again = %t, %v; want no change", changed, err)
	}

	if _, err := s.SetUnread(ctx, "missing", 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetUnread(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetPinned_PinsAndUnpins(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	if err := s.SetPinned(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Pinned {
		t.Fatalf("Conversation after pin = %+v, %v; want Pinned true", c, err)
	}

	if err := s.SetPinned(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.Pinned {
		t.Fatalf("Conversation after unpin = %+v, %v; want Pinned false", c, err)
	}

	if err := s.SetPinned(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetPinned(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetArchived_ArchivesAndUnarchives(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	if err := s.SetArchived(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Archived {
		t.Fatalf("Conversation after archive = %+v, %v; want Archived true", c, err)
	}

	if err := s.SetArchived(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.Archived {
		t.Fatalf("Conversation after unarchive = %+v, %v; want Archived false", c, err)
	}

	if err := s.SetArchived(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetArchived(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetHidden_HidesAndUnhides(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	if err := s.SetHidden(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Hidden {
		t.Fatalf("Conversation after hide = %+v, %v; want Hidden true", c, err)
	}

	if err := s.SetHidden(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.Hidden {
		t.Fatalf("Conversation after unhide = %+v, %v; want Hidden false", c, err)
	}

	if err := s.SetHidden(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetHidden(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetReminder_SnoozesAndClears(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

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

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

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

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

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

func TestConversations_OrdersPinnedFirst(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "old-pinned", "Old Pinned")
	addConversation(t, s, "wa", "new", "New")
	addConversation(t, s, "wa", "new-pinned", "New Pinned")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "old-pinned", Text: "one", Created: 10},
		domain.Message{ID: "m2", ConversationID: "new", Text: "two", Created: 20},
		domain.Message{ID: "m3", ConversationID: "new-pinned", Text: "three", Created: 30},
	)

	if err := s.SetPinned(ctx, "old-pinned", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPinned(ctx, "new-pinned", true); err != nil {
		t.Fatal(err)
	}

	// "old-pinned" has the oldest activity of the three but still leads
	// "new", which is not pinned, because pinned conversations always come
	// first; within pinned or unpinned, the newest activity leads.
	got, err := s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}

	want := []string{"new-pinned", "old-pinned", "new"}
	if !slices.Equal(gotIDs, want) {
		t.Errorf("Conversations order = %v, want %v", gotIDs, want)
	}
}

func TestEnsureConversation_NeverTouchesPinnedArchivedOrHidden(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")

	// A fresh conversation starts neither pinned, archived nor hidden,
	// even when a report carries those fields set: only Organized, from a
	// dialog sync, may set pinned or archived, and nothing but SetHidden
	// ever sets hidden. Most reports, such as a live message's own
	// conversation, carry none of them.
	first := domain.Conversation{AccountID: "wa", RemoteID: "remote", Title: "Chat", Pinned: true, Archived: true, Hidden: true}
	created, _, err := s.EnsureConversation(ctx, first)
	if err != nil || created.Pinned || created.Archived || created.Hidden {
		t.Fatalf("first EnsureConversation = %+v, %v; want none of pinned, archived or hidden", created, err)
	}

	if err := s.SetPinned(ctx, created.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived(ctx, created.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHidden(ctx, created.ID, true); err != nil {
		t.Fatal(err)
	}

	// A later bare report, such as the title and member count a dialog or
	// a live message always carries, must not unpin, unarchive or unhide
	// it: a connector never reports hidden at all, and a new message
	// arriving in a hidden chat must leave it hidden.
	second := domain.Conversation{AccountID: "wa", RemoteID: "remote", Title: "Renamed", Members: 3}
	updated, _, err := s.EnsureConversation(ctx, second)
	if err != nil || !updated.Pinned || !updated.Archived || !updated.Hidden || updated.Title != "Renamed" {
		t.Fatalf("second EnsureConversation = %+v, %v; want pinned, archived and hidden to stand, title updated", updated, err)
	}
}

func TestSetOrganized_SetsBothOrReportsNoChange(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")

	if changed, err := s.SetOrganized(ctx, "chat", true, true); err != nil || !changed {
		t.Fatalf("SetOrganized(true, true) = %t, %v; want a change", changed, err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Pinned || !c.Archived {
		t.Fatalf("Conversation after SetOrganized = %+v, %v; want both set", c, err)
	}

	if changed, err := s.SetOrganized(ctx, "chat", true, true); err != nil || changed {
		t.Errorf("SetOrganized with the same values = %t, %v; want no change", changed, err)
	}

	if changed, err := s.SetOrganized(ctx, "chat", false, false); err != nil || !changed {
		t.Errorf("SetOrganized(false, false) = %t, %v; want a change", changed, err)
	}

	if _, err := s.SetOrganized(ctx, "missing", true, true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetOrganized(missing) = %v, want ErrNotFound", err)
	}
}

func TestUnreadTotal_IgnoresMutedConversations(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "in", ConversationID: "chat", Text: "one", Created: 1})

	for _, step := range []struct {
		muted bool
		want  int
	}{{true, 0}, {false, 1}} {
		if err := s.SetMuted(ctx, "chat", step.muted); err != nil {
			t.Fatal(err)
		}

		if total, err := s.UnreadTotal(ctx); err != nil || total != step.want {
			t.Errorf("UnreadTotal with muted=%t = %d, %v; want %d", step.muted, total, err, step.want)
		}
	}

	if err := s.SetMuted(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetMuted(missing) = %v, want ErrNotFound", err)
	}
}
