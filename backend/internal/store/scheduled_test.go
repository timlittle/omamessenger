package store_test

import (
	"errors"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestAddScheduledMessage_StoresAndAssignsAnID(t *testing.T) {
	s, conv := openChatStore(t)

	m, err := s.AddScheduledMessage(t.Context(), domain.ScheduledMessage{
		ConversationID: conv.ID, Text: "see you soon", SendAt: 1000, Created: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == "" {
		t.Error("expected an id to be assigned")
	}

	got, err := s.ScheduledMessage(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != m {
		t.Errorf("stored = %+v, want %+v", got, m)
	}
}

func TestScheduledMessages_OrdersBySendAt(t *testing.T) {
	s, conv := openChatStore(t)

	later, err := s.AddScheduledMessage(t.Context(), domain.ScheduledMessage{ConversationID: conv.ID, Text: "later", SendAt: 2000, Created: 1})
	if err != nil {
		t.Fatal(err)
	}
	sooner, err := s.AddScheduledMessage(t.Context(), domain.ScheduledMessage{ConversationID: conv.ID, Text: "sooner", SendAt: 1000, Created: 2})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ScheduledMessages(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != sooner.ID || got[1].ID != later.ID {
		t.Errorf("ScheduledMessages = %+v, want [%s %s]", got, sooner.ID, later.ID)
	}
}

func TestUpdateScheduledMessage_ChangesTextAndTime(t *testing.T) {
	s, conv := openChatStore(t)

	m, err := s.AddScheduledMessage(t.Context(), domain.ScheduledMessage{ConversationID: conv.ID, Text: "draft", SendAt: 1000, Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := s.UpdateScheduledMessage(t.Context(), m.ID, "final text", 2000)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Text != "final text" || updated.SendAt != 2000 {
		t.Errorf("updated = %+v, want text=%q sendAt=2000", updated, "final text")
	}
	if updated.ConversationID != conv.ID || updated.Created != 1 {
		t.Errorf("updated kept wrong identity: %+v", updated)
	}
}

func TestUpdateScheduledMessage_NotFoundOnceClaimed(t *testing.T) {
	s, conv := openChatStore(t)

	m, err := s.AddScheduledMessage(t.Context(), domain.ScheduledMessage{ConversationID: conv.ID, Text: "draft", SendAt: 1000, Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ClaimScheduledMessage(t.Context(), m.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpdateScheduledMessage(t.Context(), m.ID, "too late", 2000); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdateScheduledMessage after claim: err = %v, want not found", err)
	}
}

func TestCancelScheduledMessage_RemovesIt(t *testing.T) {
	s, conv := openChatStore(t)

	m, err := s.AddScheduledMessage(t.Context(), domain.ScheduledMessage{ConversationID: conv.ID, Text: "draft", SendAt: 1000, Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.CancelScheduledMessage(t.Context(), m.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ScheduledMessage(t.Context(), m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ScheduledMessage after cancel: err = %v, want not found", err)
	}
}

func TestCancelScheduledMessage_NotFoundForUnknownID(t *testing.T) {
	s, _ := openChatStore(t)

	if err := s.CancelScheduledMessage(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("CancelScheduledMessage: err = %v, want not found", err)
	}
}

func TestClaimScheduledMessage_ClaimsOnceOnly(t *testing.T) {
	s, conv := openChatStore(t)

	m, err := s.AddScheduledMessage(t.Context(), domain.ScheduledMessage{ConversationID: conv.ID, Text: "draft", SendAt: 1000, Created: 1})
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := s.ClaimScheduledMessage(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("expected the first claim to succeed")
	}

	claimedAgain, err := s.ClaimScheduledMessage(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimedAgain {
		t.Error("expected a second claim of the same message to fail")
	}
}

func TestClaimScheduledMessage_FalseForUnknownID(t *testing.T) {
	s, _ := openChatStore(t)

	claimed, err := s.ClaimScheduledMessage(t.Context(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Error("expected claiming an unknown id to report false")
	}
}
