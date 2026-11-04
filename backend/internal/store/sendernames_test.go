package store_test

import (
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestRefreshSenderName_UpdatesTheGroupPreviewOnceTheSenderResolves
// reproduces a group whose preview still names its newest message's
// sender by a stale, generic label, because a connector only learns
// that sender's real name after the message was already stored (see
// bumpConversation, which sets preview_sender once and never again).
// RefreshSenderName is what a connector calls once a contact, push or
// business name resolves, so the group's preview catches up without
// waiting for another message to arrive.
func TestRefreshSenderName_UpdatesTheGroupPreviewOnceTheSenderResolves(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	group := addConversation(t, s, "wa", "group", "Weekend plans")
	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: group.ID, RemoteID: "r1",
		SenderID: "r-priya", SenderName: "WhatsApp user", Text: "Thanks", Created: 100,
	})

	changed, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair", true)
	if err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}
	if !slices.Contains(changed, group.ID) {
		t.Errorf("RefreshSenderName changed = %v, want it to include %q", changed, group.ID)
	}

	updated, err := s.Conversation(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PreviewSender != "Priya Nair" {
		t.Errorf("conversation.PreviewSender = %q, want %q", updated.PreviewSender, "Priya Nair")
	}
	if !updated.PreviewSenderVerified {
		t.Error("conversation.PreviewSenderVerified = false, want true once RefreshSenderName resolves a verified name")
	}

	stored, err := s.MessageByRemote(ctx, group.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SenderName != "Priya Nair" {
		t.Errorf("message.SenderName = %q, want it refreshed to %q too", stored.SenderName, "Priya Nair")
	}
}

// TestRefreshSenderName_LeavesAnOlderMessagesPreviewAlone confirms that
// when the sender being refreshed is not who sent the conversation's
// newest message, its own sender name is still corrected, but the
// conversation's preview, which already names whoever sent the newest
// message, is left exactly as it was.
func TestRefreshSenderName_LeavesAnOlderMessagesPreviewAlone(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	group := addConversation(t, s, "wa", "group", "Weekend plans")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: group.ID, RemoteID: "r1", SenderID: "r-priya", SenderName: "WhatsApp user", Text: "Thanks", Created: 100},
		domain.Message{ID: "m2", ConversationID: group.ID, RemoteID: "r2", SenderID: "r-sam", SenderName: "Sam", Text: "Sounds good", Created: 200},
	)

	changed, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair", true)
	if err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("RefreshSenderName changed = %v, want no conversation whose preview needed to change", changed)
	}

	updated, err := s.Conversation(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PreviewSender != "Sam" {
		t.Errorf("conversation.PreviewSender = %q, want the newest message's own sender left alone", updated.PreviewSender)
	}

	stored, err := s.MessageByRemote(ctx, group.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SenderName != "Priya Nair" {
		t.Errorf("message.SenderName = %q, want the older message's own sender name refreshed regardless", stored.SenderName)
	}
}

// TestRefreshSenderName_CorrectsAReplyQuoteToo reproduces a sent
// reply's quote showing a contact's generic fallback name instead of
// their real one: a reply keeps its own copy of the quoted message's
// sender name, resolved once when the reply was first stored (see
// resolveReply), so correcting the sender's own messages is not enough
// on its own. It also confirms a reply quoting someone else, and one
// already showing the right name, are both left alone.
func TestRefreshSenderName_CorrectsAReplyQuoteToo(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	chat := addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s,
		domain.Message{
			ID: "m1", ConversationID: chat.ID, RemoteID: "r1",
			SenderID: "r-priya", SenderName: "WhatsApp user", Text: "I'm about to try", Created: 100,
		},
		domain.Message{
			ID: "m2", ConversationID: chat.ID, RemoteID: "r2",
			SenderID: "r-sam", SenderName: "Sam", Text: "me too", Created: 150,
			ReplyTo: &domain.Reply{RemoteID: "r1", SenderName: "WhatsApp user", Text: "I'm about to try"},
		},
		domain.Message{
			ID: "m3", ConversationID: chat.ID, RemoteID: "r3",
			SenderID: "self", SenderName: "You", Text: "sure", Created: 200,
			ReplyTo: &domain.Reply{RemoteID: "r1", SenderName: "WhatsApp user", Text: "I'm about to try"},
		},
		domain.Message{
			ID: "m4", ConversationID: chat.ID, RemoteID: "r4",
			SenderID: "self", SenderName: "You", Text: "ok", Created: 250,
			ReplyTo: &domain.Reply{RemoteID: "r2", SenderName: "Sam", Text: "me too"},
		},
	)

	if _, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair", true); err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}

	quoting, err := s.Message(ctx, "m3")
	if err != nil {
		t.Fatal(err)
	}
	if quoting.ReplyTo == nil || quoting.ReplyTo.SenderName != "Priya Nair" || quoting.ReplyTo.Text != "I'm about to try" {
		t.Errorf("m3.ReplyTo = %+v, want its quoted sender corrected to Priya Nair, text kept", quoting.ReplyTo)
	}

	other, err := s.Message(ctx, "m4")
	if err != nil {
		t.Fatal(err)
	}
	if other.ReplyTo == nil || other.ReplyTo.SenderName != "Sam" {
		t.Errorf("m4.ReplyTo = %+v, want the quote of a different sender left alone", other.ReplyTo)
	}

	// Refreshing to the same name again is a no-op, not an error.
	if _, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair", true); err != nil {
		t.Fatalf("RefreshSenderName (repeat): %v", err)
	}
}

// TestRefreshSenderName_UpdatesSenderVerifiedOnEveryMessage reproduces
// a sender first stored unverified - a bare push name - whose contact
// then resolves: every message they already sent must flip to
// verified along with the corrected name, not keep showing "~" next
// to a name the UI would otherwise trust once it is corrected (see
// docs/decisions.md).
func TestRefreshSenderName_UpdatesSenderVerifiedOnEveryMessage(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	chat := addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: chat.ID, RemoteID: "r1", SenderID: "r-priya", SenderName: "WhatsApp user", SenderVerified: false, Text: "hi", Created: 100},
		domain.Message{ID: "m2", ConversationID: chat.ID, RemoteID: "r2", SenderID: "r-sam", SenderName: "Sam", SenderVerified: true, Text: "yo", Created: 150},
	)

	if _, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair", true); err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}

	refreshed, err := s.Message(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed.SenderVerified {
		t.Errorf("m1.SenderVerified = false after a verified refresh, want true")
	}

	other, err := s.Message(ctx, "m2")
	if err != nil {
		t.Fatal(err)
	}
	if !other.SenderVerified {
		t.Errorf("m2.SenderVerified = %t, want the other sender's own message left alone (still verified)", other.SenderVerified)
	}
}

// TestRefreshSenderName_CorrectsAReplyQuotesVerifiedFlagToo confirms a
// reply quote's own copy of its sender's verified flag is corrected
// the same way its SenderName already is (see
// TestRefreshSenderName_CorrectsAReplyQuoteToo): a quote resolved while
// its sender was still a bare push name must stop showing "~" once a
// contact resolves for them.
func TestRefreshSenderName_CorrectsAReplyQuotesVerifiedFlagToo(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	chat := addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s,
		domain.Message{
			ID: "m1", ConversationID: chat.ID, RemoteID: "r1",
			SenderID: "r-priya", SenderName: "WhatsApp user", SenderVerified: false, Text: "I'm about to try", Created: 100,
		},
		domain.Message{
			ID: "m2", ConversationID: chat.ID, RemoteID: "r2",
			SenderID: "self", SenderName: "You", SenderVerified: true, Text: "sure", Created: 200,
			ReplyTo: &domain.Reply{RemoteID: "r1", SenderName: "WhatsApp user", SenderVerified: false, Text: "I'm about to try"},
		},
	)

	if _, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair", true); err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}

	quoting, err := s.Message(ctx, "m2")
	if err != nil {
		t.Fatal(err)
	}
	if quoting.ReplyTo == nil || !quoting.ReplyTo.SenderVerified {
		t.Errorf("m2.ReplyTo = %+v, want its quoted sender's verified flag corrected to true", quoting.ReplyTo)
	}
}

// TestRefreshSenderName_IgnoresASenderWithNoMessagesStored confirms
// refreshing a sender nothing has ever quoted or sent a message under
// changes nothing and returns no error.
func TestRefreshSenderName_IgnoresASenderWithNoMessagesStored(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	chat := addConversation(t, s, "wa", "chat", "Chat")
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: chat.ID, RemoteID: "r1", SenderID: "r-sam", SenderName: "Sam", Text: "hi", Created: 100})

	changed, err := s.RefreshSenderName(ctx, "wa", "r-nobody", "Nobody", true)
	if err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("RefreshSenderName changed = %v, want none", changed)
	}
}

// TestRefreshSenderName_IsScopedToOneAccount confirms a sender id that
// happens to match one used by a different account's conversation is
// never touched: remote ids are only unique within a service account.
func TestRefreshSenderName_IsScopedToOneAccount(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addAccount(t, s, "wa2")
	group := addConversation(t, s, "wa2", "group", "Other account's group")
	addMessages(t, s, domain.Message{
		ID: "m1", ConversationID: group.ID, RemoteID: "r1",
		SenderID: "r-priya", SenderName: "WhatsApp user", Text: "Thanks", Created: 100,
	})

	changed, err := s.RefreshSenderName(ctx, "wa", "r-priya", "Priya Nair", true)
	if err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("RefreshSenderName changed = %v, want another account's conversation left untouched", changed)
	}

	stored, err := s.MessageByRemote(ctx, group.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.SenderName != "WhatsApp user" {
		t.Errorf("message.SenderName = %q, want another account's message left untouched", stored.SenderName)
	}
}

// TestRefreshSenderName_NeverRelabelsAQuoteFromAnotherConversationsReusedRemoteID
// reproduces the WhatsApp scenario where a reused message id let a
// rename reach a quote it never actually named: Alice posts message X
// in group G; Bob replies there, quoting it. Mallory, who is also in
// G, separately reuses the same remote id X on a message of her own
// in a direct chat with the user (a message id is only unique within
// one conversation, not the whole account, so WhatsApp's own reused-id
// guard, keyed per conversation, does not catch this). Mallory also
// posts in G under her own id, so the account-wide rename below does
// reach replies in G - but Bob's quote names Alice's id, stored when
// it was first resolved within G itself (see resolveReply), never
// Mallory's, so renaming Mallory must still leave it alone.
func TestRefreshSenderName_NeverRelabelsAQuoteFromAnotherConversationsReusedRemoteID(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	group := addConversation(t, s, "wa", "group", "Group")
	dm := addConversation(t, s, "wa", "dm", "Mallory")
	addMessages(t, s,
		domain.Message{ID: "alice-x", ConversationID: group.ID, RemoteID: "x", SenderID: "r-alice", SenderName: "Alice", Text: "let's meet at noon", Created: 100},
		domain.Message{ID: "bob-quote", ConversationID: group.ID, RemoteID: "r2", SenderID: "r-bob", SenderName: "Bob", Text: "works for me", Created: 150, ReplyTo: &domain.Reply{RemoteID: "x"}},
		domain.Message{ID: "mallory-in-group", ConversationID: group.ID, RemoteID: "r3", SenderID: "r-mallory", SenderName: "WhatsApp user", Text: "hi all", Created: 160},
		domain.Message{ID: "mallory-x", ConversationID: dm.ID, RemoteID: "x", SenderID: "r-mallory", SenderName: "WhatsApp user", Text: "hey", Created: 170},
	)

	if _, err := s.RefreshSenderName(ctx, "wa", "r-mallory", "Bob", true); err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}

	quoting, err := s.Message(ctx, "bob-quote")
	if err != nil {
		t.Fatal(err)
	}
	if quoting.ReplyTo == nil || quoting.ReplyTo.SenderName != "Alice" {
		t.Errorf("bob-quote.ReplyTo = %+v, want Alice's quote left alone, not relabelled Bob", quoting.ReplyTo)
	}
}

// TestRefreshSenderName_IgnoresANaturallyCollidingRemoteIDInAnotherConversation
// reproduces Telegram's own natural id collisions: message ids are
// only unique within one chat, so two unrelated chats can each have a
// message numbered the same with no crafted input needed. A rename in
// one chat must never reach a quote in the other just because their
// quoted messages happen to share a remote id.
func TestRefreshSenderName_IgnoresANaturallyCollidingRemoteIDInAnotherConversation(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	if err := s.UpsertAccount(ctx, domain.Account{ID: "tg", Service: domain.ServiceTelegram, Name: "tg"}); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}
	chatA := addConversation(t, s, "tg", "chatA", "Chat A")
	chatB := addConversation(t, s, "tg", "chatB", "Chat B")
	addMessages(t, s,
		domain.Message{ID: "a-42", ConversationID: chatA.ID, RemoteID: "42", SenderID: "r-nina", SenderName: "Nina", Text: "see you then", Created: 100},
		domain.Message{ID: "a-quote", ConversationID: chatA.ID, RemoteID: "43", SenderID: "r-omar", SenderName: "Omar", Text: "great", Created: 150, ReplyTo: &domain.Reply{RemoteID: "42"}},
		domain.Message{ID: "b-42", ConversationID: chatB.ID, RemoteID: "42", SenderID: "r-priya", SenderName: "Priya", Text: "unrelated chat", Created: 200},
	)

	if _, err := s.RefreshSenderName(ctx, "tg", "r-priya", "Priya Nair", true); err != nil {
		t.Fatalf("RefreshSenderName: %v", err)
	}

	quoting, err := s.Message(ctx, "a-quote")
	if err != nil {
		t.Fatal(err)
	}
	if quoting.ReplyTo == nil || quoting.ReplyTo.SenderName != "Nina" {
		t.Errorf("a-quote.ReplyTo = %+v, want Nina's quote in chat A left alone by a rename in chat B", quoting.ReplyTo)
	}
}
