package app_test

import (
	"maps"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestUnreadChanged_BreaksDownUnreadByService proves an unread.changed
// event's ByService field counts each service's own unread messages, so
// the bar tooltip can show WhatsApp and Telegram apart.
func TestUnreadChanged_BreaksDownUnreadByService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	if err := f.store.UpsertAccount(ctx, domain.Account{ID: "tg", Service: domain.ServiceTelegram, Name: "tg"}); err != nil {
		t.Fatal(err)
	}
	tgChat, _, err := f.store.EnsureConversation(ctx, domain.Conversation{
		ID: "tg-chat", AccountID: "tg", RemoteID: "r-tg-chat", Title: "TG Chat", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.published.take()

	f.ingest.Unread(ctx, "wa", chat.RemoteID, 2)
	f.ingest.Unread(ctx, "tg", tgChat.RemoteID, 1)

	want := app.UnreadChanged{Total: 3, ByService: map[string]int{domain.ServiceWhatsApp: 2, domain.ServiceTelegram: 1}}
	got, ok := f.published.lastOf(app.EventUnreadChanged).(app.UnreadChanged)
	if !ok || got.Total != want.Total || !maps.Equal(got.ByService, want.ByService) {
		t.Errorf("unread.changed = %+v, want %+v", got, want)
	}
}
