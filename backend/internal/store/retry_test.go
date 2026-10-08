package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMessageRetry_SchedulesStopsAndClears confirms the three retry
// state transitions the automatic retry scheduler relies on:
// scheduling a next attempt, stopping it with the attempt count kept,
// and clearing it entirely.
func TestMessageRetry_SchedulesStopsAndClears(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "out", ConversationID: "chat", Text: "hi", Outgoing: true, Status: domain.StatusFailed, Created: 1})

	if err := s.ScheduleMessageRetry(ctx, "out", 500, 2, 100); err != nil {
		t.Fatalf("ScheduleMessageRetry() error = %v", err)
	}

	got, err := s.Message(ctx, "out")
	if err != nil || got.RetryAt != 500 || got.RetryAttempts != 2 || got.RetrySince != 100 {
		t.Fatalf("after schedule = %+v, %v", got, err)
	}

	if err := s.StopMessageRetry(ctx, "out"); err != nil {
		t.Fatalf("StopMessageRetry() error = %v", err)
	}

	got, err = s.Message(ctx, "out")
	if err != nil || got.RetryAt != 0 || got.RetryAttempts != 2 || got.RetrySince != 100 {
		t.Fatalf("after stop = %+v, %v, want only retryAt cleared", got, err)
	}

	if err := s.ScheduleMessageRetry(ctx, "out", 999, 3, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearMessageRetry(ctx, "out"); err != nil {
		t.Fatalf("ClearMessageRetry() error = %v", err)
	}

	got, err = s.Message(ctx, "out")
	if err != nil || got.RetryAt != 0 || got.RetryAttempts != 0 || got.RetrySince != 0 {
		t.Fatalf("after clear = %+v, %v, want everything reset", got, err)
	}
}

// TestPendingRetries_OrdersBySoonestDueAndIgnoresOthers confirms
// PendingRetries returns only failed outgoing messages with a retry
// scheduled, soonest due first, leaving out one with nothing scheduled
// and one that is not failed at all.
func TestPendingRetries_OrdersBySoonestDueAndIgnoresOthers(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s,
		domain.Message{ID: "later", ConversationID: "chat", Text: "a", Outgoing: true, Status: domain.StatusFailed, Created: 1},
		domain.Message{ID: "sooner", ConversationID: "chat", Text: "b", Outgoing: true, Status: domain.StatusFailed, Created: 2},
		domain.Message{ID: "unscheduled", ConversationID: "chat", Text: "c", Outgoing: true, Status: domain.StatusFailed, Created: 3},
		domain.Message{ID: "sent", ConversationID: "chat", Text: "d", Outgoing: true, Status: domain.StatusSent, Created: 4},
	)

	if err := s.ScheduleMessageRetry(ctx, "later", 2000, 1, 1000); err != nil {
		t.Fatal(err)
	}
	if err := s.ScheduleMessageRetry(ctx, "sooner", 1000, 1, 1000); err != nil {
		t.Fatal(err)
	}

	pending, err := s.PendingRetries(ctx)
	if err != nil {
		t.Fatalf("PendingRetries() error = %v", err)
	}

	var ids []string
	for _, m := range pending {
		ids = append(ids, m.ID)
	}
	if !slices.Equal(ids, []string{"sooner", "later"}) {
		t.Errorf("PendingRetries() ids = %v, want [sooner later]", ids)
	}
}

// TestMessageExists_ReportsStoredAndMissingMessages confirms
// MessageExists distinguishes a message that is stored from one that
// is not, the check the outgoing media area's sweep relies on.
func TestMessageExists_ReportsStoredAndMissingMessages(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "out", ConversationID: "chat", Text: "hi", Created: 1})

	if exists, err := s.MessageExists(ctx, "out"); err != nil || !exists {
		t.Errorf("MessageExists(stored) = %t, %v, want true, nil", exists, err)
	}
	if exists, err := s.MessageExists(ctx, "missing"); err != nil || exists {
		t.Errorf("MessageExists(missing) = %t, %v, want false, nil", exists, err)
	}
}

// TestFailedAttachmentCount_CountsOnlyFailedOutgoingMessagesWithMedia
// confirms the doctor's outgoing-area warning counts only what it
// means to: a failed, outgoing message that still carries an
// attachment, not a sent one, an incoming one, or one with no media.
func TestFailedAttachmentCount_CountsOnlyFailedOutgoingMessagesWithMedia(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	media := &domain.Media{Kind: domain.MediaPhoto, FileName: "photo.png"}
	addMessages(t, s,
		domain.Message{ID: "failed-with-media", ConversationID: "chat", Text: "a", Outgoing: true, Status: domain.StatusFailed, Created: 1, Media: media},
		domain.Message{ID: "failed-no-media", ConversationID: "chat", Text: "b", Outgoing: true, Status: domain.StatusFailed, Created: 2},
		domain.Message{ID: "sent-with-media", ConversationID: "chat", Text: "c", Outgoing: true, Status: domain.StatusSent, Created: 3, Media: media},
		domain.Message{ID: "incoming", ConversationID: "chat", Text: "d", Created: 4, Media: media},
	)

	n, err := s.FailedAttachmentCount(ctx)
	if err != nil || n != 1 {
		t.Errorf("FailedAttachmentCount() = %d, %v, want 1", n, err)
	}
}

// TestDeleteMessagesByID_RemovesMessagesWithNoRemoteID confirms
// DeleteMessagesByID removes messages found by their own local ids -
// the only way to delete a failed send, which has no remote id for
// DeleteMessages to match - and brings their conversation's preview and
// unread count up to date, the same as DeleteMessages.
func TestDeleteMessagesByID_RemovesMessagesWithNoRemoteID(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "failed", ConversationID: "chat", Text: "hi", Outgoing: true, Status: domain.StatusFailed, Created: 1})

	deleted, err := s.DeleteMessagesByID(ctx, []string{"failed"})
	if err != nil || len(deleted) != 1 || deleted[0].ID != "failed" {
		t.Fatalf("DeleteMessagesByID() = %+v, %v", deleted, err)
	}

	if _, err := s.Message(ctx, "failed"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Message after delete = %v, want ErrNotFound", err)
	}
}

// TestDeleteMessagesByID_ToleratesNoIDs confirms an empty id list is not
// an error, the same as DeleteMessages with no remote ids.
func TestDeleteMessagesByID_ToleratesNoIDs(t *testing.T) {
	t.Parallel()

	s := openStore(t)

	deleted, err := s.DeleteMessagesByID(t.Context(), nil)
	if err != nil || deleted != nil {
		t.Errorf("DeleteMessagesByID(nil) = %v, %v, want nil, nil", deleted, err)
	}
}
