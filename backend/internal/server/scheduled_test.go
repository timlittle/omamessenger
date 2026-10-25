package server_test

import (
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMessagesSchedule_RoundTrip(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	sendAt := time.Now().Add(time.Hour).UnixMilli()

	queued, err := call[domain.ScheduledMessage](t, s, "messages.schedule", map[string]any{
		"conversationId": "chat", "text": "see you soon", "sendAt": sendAt,
	})
	if err != nil || queued.Text != "see you soon" || queued.SendAt != sendAt {
		t.Fatalf("messages.schedule = %+v, %v", queued, err)
	}

	listed, err := call[struct {
		Messages []domain.ScheduledMessage `json:"messages"`
	}](t, s, "messages.listScheduled", map[string]string{"conversationId": "chat"})
	if err != nil || len(listed.Messages) != 1 || listed.Messages[0].ID != queued.ID {
		t.Fatalf("messages.listScheduled = %+v, %v", listed, err)
	}

	newSendAt := time.Now().Add(2 * time.Hour).UnixMilli()
	edited, err := call[domain.ScheduledMessage](t, s, "messages.editScheduled", map[string]any{
		"scheduledId": queued.ID, "text": "see you a bit later", "sendAt": newSendAt,
	})
	if err != nil || edited.Text != "see you a bit later" || edited.SendAt != newSendAt {
		t.Fatalf("messages.editScheduled = %+v, %v", edited, err)
	}

	sent, err := call[domain.Message](t, s, "messages.sendScheduledNow", map[string]string{"scheduledId": queued.ID})
	if err != nil || sent.Text != "see you a bit later" {
		t.Fatalf("messages.sendScheduledNow = %+v, %v", sent, err)
	}

	listedAfter, err := call[struct {
		Messages []domain.ScheduledMessage `json:"messages"`
	}](t, s, "messages.listScheduled", nil)
	if err != nil || len(listedAfter.Messages) != 0 {
		t.Fatalf("messages.listScheduled after sendScheduledNow = %+v, %v, want none left queued", listedAfter, err)
	}
}

func TestMessagesCancelScheduled_RemovesIt(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	queued, err := call[domain.ScheduledMessage](t, s, "messages.schedule", map[string]any{
		"conversationId": "chat", "text": "never mind", "sendAt": time.Now().Add(time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := call[struct{}](t, s, "messages.cancelScheduled", map[string]string{"scheduledId": queued.ID}); err != nil {
		t.Fatalf("messages.cancelScheduled = %v", err)
	}

	if _, err := call[struct{}](t, s, "messages.cancelScheduled", map[string]string{"scheduledId": queued.ID}); code(err) == 0 {
		t.Error("messages.cancelScheduled a second time succeeded, want not found")
	}
}

func TestMessagesSchedule_RejectsATimeNotInTheFuture(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	if _, err := call[domain.ScheduledMessage](t, s, "messages.schedule", map[string]any{
		"conversationId": "chat", "text": "too late", "sendAt": time.Now().Add(-time.Hour).UnixMilli(),
	}); code(err) == 0 {
		t.Error("messages.schedule with a past time succeeded")
	}
}
