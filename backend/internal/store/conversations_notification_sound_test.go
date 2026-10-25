package store_test

import (
	"errors"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSetNotificationSound_SetsAndClearsCustom(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.NotificationSound != "" {
		t.Fatalf("Conversation before any change = %+v, %v; want notification sound \"\"", c, err)
	}

	if err := s.SetNotificationSound(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.NotificationSound != "custom" {
		t.Fatalf("Conversation after SetNotificationSound(true) = %+v, %v; want \"custom\"", c, err)
	}

	if err := s.SetNotificationSound(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.NotificationSound != "" {
		t.Fatalf("Conversation after SetNotificationSound(false) = %+v, %v; want \"\"", c, err)
	}
}

func TestSetNotificationSound_NotFound(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)

	if err := s.SetNotificationSound(t.Context(), "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetNotificationSound(missing) = %v, want ErrNotFound", err)
	}
}
