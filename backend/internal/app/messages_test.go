package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMessages_ValidatesInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	page, more, err := f.commands.Messages(ctx, "chat", "", 0)
	if err != nil || len(page) != 0 || more {
		t.Fatalf("Messages(empty chat) = %v, %t, %v", page, more, err)
	}

	tests := []struct {
		name           string
		conversationID string
		limit          int
		want           error
	}{
		{"no conversation", " ", 10, app.ErrInvalidInput},
		{"negative limit", "chat", -1, app.ErrInvalidInput},
		{"limit too large", "chat", app.MaxPageSize + 1, app.ErrInvalidInput},
		{"unknown conversation", "missing", 10, domain.ErrNotFound},
	}

	for _, tt := range tests {
		if _, _, err := f.commands.Messages(ctx, tt.conversationID, "", tt.limit); !errors.Is(err, tt.want) {
			t.Errorf("%s: Messages = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestSend_StoresPublishesAndDispatches(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	m, err := f.commands.Send(t.Context(), "chat", "  send this  ", "")
	if err != nil || m.Text != "send this" || !m.Outgoing || m.Status != domain.StatusPending {
		t.Fatalf("Send = %+v, %v", m, err)
	}

	if !slices.Equal(f.dispatcher.sent, []string{"send this"}) {
		t.Errorf("dispatched = %v", f.dispatcher.sent)
	}

	want := []string{app.EventMessageAdded, app.EventConversationUpdated}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestSend_WithReplyToFillsTheQuoteAndPassesItToTheConnector(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	quoted, err := f.commands.Send(t.Context(), "chat", "original", "")
	if err != nil {
		t.Fatal(err)
	}

	reply, err := f.commands.Send(t.Context(), "chat", "sure", quoted.ID)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.SenderName != "You" || reply.ReplyTo.Text != "original" {
		t.Fatalf("reply.ReplyTo = %+v, want it to quote the first message", reply.ReplyTo)
	}

	if sent := f.dispatcher.last(); sent.ReplyTo == nil || sent.ReplyTo.Text != "original" {
		t.Errorf("dispatched message's ReplyTo = %+v, want the quote", sent.ReplyTo)
	}
}

func TestSend_RejectsReplyToAMessageInAnotherConversation(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.conversation(t, "other", "Other", domain.KindDirect)

	elsewhere, err := f.commands.Send(t.Context(), "other", "hi", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.commands.Send(t.Context(), "chat", "sure", elsewhere.ID); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Send with a reply from another conversation = %v, want ErrInvalidInput", err)
	}
}

func TestSend_RejectsReplyToAnUnknownMessage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	if _, err := f.commands.Send(t.Context(), "chat", "sure", "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Send with an unknown reply target = %v, want ErrNotFound", err)
	}
}

func TestSend_RefusedMessageIsFailedNotAnError(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.dispatcher.err = errors.New("offline")

	m, err := f.commands.Send(t.Context(), "chat", "hello", "")
	if err != nil || m.Status != domain.StatusFailed {
		t.Fatalf("Send = %+v, %v; want a failed message", m, err)
	}

	stored, err := f.store.Message(t.Context(), m.ID)
	if err != nil || stored.Status != domain.StatusFailed {
		t.Errorf("stored = %+v, %v", stored, err)
	}

	want := []string{app.EventMessageAdded, app.EventConversationUpdated, app.EventMessageUpdated}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestSend_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	tests := []struct {
		name, conversationID, text string
		want                       error
	}{
		{"blank text", "chat", "  ", app.ErrInvalidInput},
		{"text too long", "chat", strings.Repeat("a", domain.MaxTextLength+1), app.ErrInvalidInput},
		{"unknown conversation", "missing", "text", domain.ErrNotFound},
	}

	for _, tt := range tests {
		if _, err := f.commands.Send(ctx, tt.conversationID, tt.text, ""); !errors.Is(err, tt.want) {
			t.Errorf("%s: Send = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestRetry_SendsAFailedMessageAgain(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.dispatcher.err = errors.New("offline")

	failed, _ := f.commands.Send(ctx, "chat", "hello", "")
	f.published.take()

	f.dispatcher.err = nil
	f.dispatcher.onRun = func(m domain.Message) {
		f.ingest.OutgoingStatus(ctx, m.ID, "remote-1", domain.StatusSent)
	}

	retried, err := f.commands.Retry(ctx, failed.ID)
	if err != nil || retried.Status != domain.StatusPending {
		t.Fatalf("Retry = %+v, %v", retried, err)
	}

	stored, _ := f.store.Message(ctx, failed.ID)
	if stored.Status != domain.StatusSent || stored.RemoteID != "remote-1" {
		t.Errorf("stored = %+v", stored)
	}

	want := []string{app.EventMessageUpdated, app.EventMessageUpdated}
	if got := f.published.take(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}

	if _, err := f.commands.Retry(ctx, failed.ID); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("Retry(sent message) = %v, want ErrInvalidInput", err)
	}

	if _, err := f.commands.Retry(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Retry(missing) = %v, want ErrNotFound", err)
	}
}

func TestMessages_LoadsOlderHistoryFromTheService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	for _, m := range []domain.Message{
		{ID: "m50", ConversationID: "chat", RemoteID: "50", Text: "x", Created: 50},
		{ID: "m60", ConversationID: "chat", RemoteID: "60", Text: "y", Created: 60},
	} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	f.history.older = []domain.Message{
		{ID: "m30", RemoteID: "30", Text: "c", Created: 30},
		{ID: "m20", RemoteID: "20", Text: "b", Created: 20},
	}

	page, more, err := f.commands.Messages(ctx, "chat", "m50", 10)
	if err != nil || len(page) != 2 || page[0].ID != "m20" || !more {
		t.Fatalf("Messages before m50 = %v, more %t, %v; want the two older ones, and maybe more", page, more, err)
	}

	page, more, err = f.commands.Messages(ctx, "chat", "m20", 10)
	if err != nil || len(page) != 0 || more {
		t.Errorf("Messages before m20 = %v, more %t, %v; want the end of history", page, more, err)
	}

	if !slices.Equal(f.history.from, []string{"50", "20"}) {
		t.Errorf("loaded from %v, want [50 20]", f.history.from)
	}
}

func TestMessages_KeepsWhatItHasWhenTheServiceFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.history.err = errors.New("offline")

	page, more, err := f.commands.Messages(t.Context(), "chat", "", 10)
	if err != nil || len(page) != 0 || more {
		t.Errorf("Messages with the service offline = %v, %t, %v; want the local page and no error", page, more, err)
	}
}

func TestMessages_FillsInMediaTheServiceReportedTooLateToStore(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	for _, m := range []domain.Message{
		{ID: "m1", ConversationID: "chat", RemoteID: "40", Text: "[Photo]", Created: 1},
		{ID: "m2", ConversationID: "chat", RemoteID: "41", Text: "see https://x.io/a", Created: 2},
		{ID: "m3", ConversationID: "chat", RemoteID: "42", Text: "already has media", Created: 3, Media: &domain.Media{Kind: domain.MediaPhoto}},
	} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	photo := domain.Media{Kind: domain.MediaPhoto, Width: 10, Height: 10}
	link := domain.Media{Kind: domain.MediaLink, URL: "https://x.io/a"}
	f.refresher.toSend = map[string]domain.Message{
		"40": {RemoteID: "40", Text: "[Photo]", Created: 1, Media: &photo},
		"41": {RemoteID: "41", Text: "see https://x.io/a", Created: 2, Media: &link},
	}

	page, _, err := f.commands.Messages(ctx, chat.ID, "", 10)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]domain.Message{}
	for _, m := range page {
		byID[m.ID] = m
	}

	if m := byID["m1"]; m.Media == nil || m.Media.Kind != domain.MediaPhoto {
		t.Errorf("m1 media = %+v, want the photo filled in", m.Media)
	}
	if m := byID["m2"]; m.Media == nil || m.Media.Kind != domain.MediaLink {
		t.Errorf("m2 media = %+v, want the link preview filled in", m.Media)
	}

	if want := [][]string{{"40", "41"}}; !slices.EqualFunc(f.refresher.asked, want, slices.Equal) {
		t.Errorf("asked to refresh %v, want [[40 41]]", f.refresher.asked)
	}

	// Looking at the same page again must not ask the service a second
	// time for messages already attempted.
	if _, _, err := f.commands.Messages(ctx, chat.ID, "", 10); err != nil {
		t.Fatal(err)
	}
	if len(f.refresher.asked) != 1 {
		t.Errorf("asked to refresh again: %v, want only the first attempt", f.refresher.asked)
	}
}

func TestMessages_KeepsTheStoredPageWhenRefreshingFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{ID: "m1", ConversationID: "chat", RemoteID: "40", Text: "[Photo]", Created: 1}); err != nil {
		t.Fatal(err)
	}
	f.refresher.err = errors.New("offline")

	page, more, err := f.commands.Messages(ctx, chat.ID, "", 10)
	if err != nil || more || len(page) != 1 || page[0].ID != "m1" || page[0].Media != nil {
		t.Fatalf("Messages with the refresher offline = %v, %t, %v; want the stored page unchanged", page, more, err)
	}
}

func TestMessages_OlderHistoryDoesNotCountAsUnread(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.history.older = []domain.Message{
		{ID: "m30", RemoteID: "30", Text: "c", Created: 30, Status: domain.StatusReceived},
		{ID: "m20", RemoteID: "20", Text: "b", Created: 20, Status: domain.StatusReceived},
	}

	if _, _, err := f.commands.Messages(ctx, chat.ID, "", 10); err != nil {
		t.Fatal(err)
	}

	if got, _ := f.store.Conversation(ctx, chat.ID); got.Unread != 0 {
		t.Errorf("unread = %d after loading older history, want 0", got.Unread)
	}

	// Storing the older messages announced higher totals on the way; the
	// window must hear the corrected one last.
	if got, ok := f.published.lastOf(app.EventUnreadChanged).(app.UnreadChanged); !ok || got.Total != 0 {
		t.Errorf("last unread total published = %+v, want 0", got)
	}
}
