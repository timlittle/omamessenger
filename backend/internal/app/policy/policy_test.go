package policy_test

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app/policy"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// everyDetail lists every notification detail level, for tests that must
// cover all three.
var everyDetail = []policy.Detail{policy.DetailNameAndMessage, policy.DetailNameOnly, policy.DetailNone}

// everyInput returns an arrival for every combination of the four flags
// and every detail level, in both a direct chat and a group.
func everyInput() []policy.Input {
	var inputs []policy.Input
	for bits := range 16 {
		for _, detail := range everyDetail {
			for _, kind := range []string{domain.KindDirect, domain.KindGroup} {
				inputs = append(inputs, policy.Input{
					Notifications:  bits&1 != 0,
					Detail:         detail,
					Muted:          bits&2 != 0,
					Focused:        bits&4 != 0,
					WindowActive:   bits&8 != 0,
					Kind:           kind,
					Sender:         "Priya",
					Title:          "Climbing Crew",
					Text:           "see you at 7",
					ConversationID: "chat-1",
				})
			}
		}
	}

	return inputs
}

func TestReminderNotification_NamesTheChatUnlessDetailIsNone(t *testing.T) {
	t.Parallel()

	title, body, convID := policy.ReminderNotification(policy.DetailNameAndMessage, "Climbing Crew", "chat-1")
	if title != "Reminder: Climbing Crew" || body == "" || convID != "chat-1" {
		t.Errorf("ReminderNotification(nameAndMessage) = %q, %q, %q", title, body, convID)
	}

	title, _, convID = policy.ReminderNotification(policy.DetailNameOnly, "Climbing Crew", "chat-1")
	if title != "Reminder: Climbing Crew" || convID != "chat-1" {
		t.Errorf("ReminderNotification(nameOnly) = %q, _, %q", title, convID)
	}

	title, body, convID = policy.ReminderNotification(policy.DetailNone, "Climbing Crew", "chat-1")
	if strings.Contains(title, "Climbing Crew") || strings.Contains(body, "Climbing Crew") {
		t.Errorf("ReminderNotification(none) = %q, %q; must not name the chat", title, body)
	}
	if convID != "chat-1" {
		t.Errorf("ReminderNotification(none) conversation id = %q, want chat-1", convID)
	}
}

func TestMarkReadOnArrival_OnlyWhenLookingAtIt(t *testing.T) {
	t.Parallel()

	for _, in := range everyInput() {
		want := in.Focused && in.WindowActive
		if got := policy.MarkReadOnArrival(in); got != want {
			t.Errorf("%+v: MarkReadOnArrival = %t, want %t", in, got, want)
		}
	}
}

func TestShouldNotify_UnlessOffMutedOrLookingAtIt(t *testing.T) {
	t.Parallel()

	for _, in := range everyInput() {
		lookingAtIt := in.Focused && in.WindowActive
		want := in.Notifications && !in.Muted && !lookingAtIt
		if got := policy.ShouldNotify(in); got != want {
			t.Errorf("%+v: ShouldNotify = %t, want %t", in, got, want)
		}
	}
}

func TestNotification_NamesGroupsAndRespectsTheDetailLevel(t *testing.T) {
	t.Parallel()

	for _, in := range everyInput() {
		wantTitle, wantBody := "Priya", "New message"
		switch in.Detail {
		case policy.DetailNameAndMessage:
			wantBody = "see you at 7"
			if in.Kind == domain.KindGroup {
				wantTitle = "Priya · Climbing Crew"
			}
		case policy.DetailNameOnly:
			if in.Kind == domain.KindGroup {
				wantTitle = "Priya · Climbing Crew"
			}
		case policy.DetailNone:
			wantTitle = "OmaMessenger"
		}

		title, body, conversationID := policy.Notification(in)
		if title != wantTitle || body != wantBody {
			t.Errorf("%+v: Notification = %q, %q; want %q, %q", in, title, body, wantTitle, wantBody)
		}
		if conversationID != in.ConversationID {
			t.Errorf("%+v: Notification conversation id = %q, want %q", in, conversationID, in.ConversationID)
		}
	}
}
