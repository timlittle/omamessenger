package app_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestIncoming_PublishesNotificationSoundOnlyWhenItNotifies confirms
// notification.sound follows exactly the same gate the desktop
// notification itself does: never for a muted conversation, and never
// for one the user is already looking at. The UI's own precedence
// between a conversation's, a service's and the global default sound
// (see ui/lib/NotificationSound.js) never even runs for either case,
// since the event that would trigger it is simply never sent.
func TestIncoming_PublishesNotificationSoundOnlyWhenItNotifies(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	if _, err := f.commands.SetMuted(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("muted", "quiet"))
	if events := f.published.take(); slices.Contains(events, app.EventNotificationSound) {
		t.Errorf("events for a muted conversation = %v, want no %s", events, app.EventNotificationSound)
	}

	if _, err := f.commands.SetMuted(ctx, chat.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := f.commands.SetFocus(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("seen", "looking"))
	if events := f.published.take(); slices.Contains(events, app.EventNotificationSound) {
		t.Errorf("events for the focused conversation = %v, want no %s", events, app.EventNotificationSound)
	}

	if err := f.commands.SetFocus(ctx, "", true); err != nil {
		t.Fatal(err)
	}
	f.ingest.Incoming(ctx, "wa", chat.RemoteID, incoming("away", "ping"))
	if events := f.published.take(); !slices.Contains(events, app.EventNotificationSound) {
		t.Errorf("events for an ordinary arrival = %v, want %s", events, app.EventNotificationSound)
	}
}
