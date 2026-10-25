package store_test

import (
	"errors"
	"maps"
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

// TestEnsureConversation_KeepsTwoAccountsOfTheSameServiceIndependent proves
// issue #12's premise at the storage layer: two accounts signed in to the
// same service (two WhatsApp numbers, say) are told apart by account id
// alone, not by service, so the same remote id - the same contact
// talking to both - gets its own conversation row under each account,
// and deleting one account's data never touches the other's.
func TestEnsureConversation_KeepsTwoAccountsOfTheSameServiceIndependent(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa-one")
	addAccount(t, s, "wa-two")

	first, isNew, err := s.EnsureConversation(ctx, domain.Conversation{
		ID: "chat-one", AccountID: "wa-one", RemoteID: "shared-remote", Title: "Shared Contact", Kind: domain.KindDirect,
	})
	if err != nil || !isNew {
		t.Fatalf("first EnsureConversation = %+v, %t, %v", first, isNew, err)
	}

	second, isNew, err := s.EnsureConversation(ctx, domain.Conversation{
		ID: "chat-two", AccountID: "wa-two", RemoteID: "shared-remote", Title: "Shared Contact", Kind: domain.KindDirect,
	})
	if err != nil || !isNew {
		t.Fatalf("second EnsureConversation = %+v, %t, %v", second, isNew, err)
	}

	if first.ID == second.ID {
		t.Fatalf("both accounts' conversations shared one row: %+v, %+v", first, second)
	}

	byOne, err := s.ConversationByRemote(ctx, "wa-one", "shared-remote")
	if err != nil || byOne.ID != "chat-one" {
		t.Fatalf("ConversationByRemote(wa-one) = %+v, %v, want chat-one", byOne, err)
	}

	byTwo, err := s.ConversationByRemote(ctx, "wa-two", "shared-remote")
	if err != nil || byTwo.ID != "chat-two" {
		t.Fatalf("ConversationByRemote(wa-two) = %+v, %v, want chat-two", byTwo, err)
	}

	if _, _, err := s.AddMessage(ctx, domain.Message{ID: "m-one", ConversationID: "chat-one", Text: "hi from one", Created: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddMessage(ctx, domain.Message{ID: "m-two", ConversationID: "chat-two", Text: "hi from two", Created: 1}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteAccount(ctx, "wa-one"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Conversation(ctx, "chat-one"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("chat-one after deleting its account = %v, want ErrNotFound", err)
	}

	stillThere, err := s.Conversation(ctx, "chat-two")
	if err != nil || stillThere.AccountID != "wa-two" {
		t.Fatalf("chat-two after deleting the other account = %+v, %v, want untouched", stillThere, err)
	}

	messages, _, err := s.Messages(ctx, "chat-two", "", 10)
	if err != nil || len(messages) != 1 || messages[0].ID != "m-two" {
		t.Fatalf("messages for chat-two after deleting the other account = %+v, %v", messages, err)
	}

	accounts, err := s.Accounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].ID != "wa-two" {
		t.Fatalf("Accounts after removing wa-one = %+v, %v, want only wa-two", accounts, err)
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

	s, _ := openChatStore(t)
	ctx := t.Context()
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

	s, _ := openChatStore(t)
	ctx := t.Context()
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

func TestSetArchived_ArchivesAndUnarchives(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

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

	s, _ := openChatStore(t)
	ctx := t.Context()

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

func TestSetBlocked_BlocksAndUnblocks(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if err := s.SetBlocked(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Blocked {
		t.Fatalf("Conversation after block = %+v, %v; want Blocked true", c, err)
	}

	if err := s.SetBlocked(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.Blocked {
		t.Fatalf("Conversation after unblock = %+v, %v; want Blocked false", c, err)
	}

	if err := s.SetBlocked(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetBlocked(missing) = %v, want ErrNotFound", err)
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

func TestUnreadTotal_IgnoresMutedConversations(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()
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

func TestUnreadByService_GroupsByServiceAndIgnoresMuted(t *testing.T) {
	t.Parallel()

	s, chat := openChatStore(t)
	ctx := t.Context()
	addMessages(t, s, domain.Message{ID: "in", ConversationID: chat.ID, Text: "one", Created: 1})

	if err := s.UpsertAccount(ctx, domain.Account{ID: "tg", Service: domain.ServiceTelegram, Name: "tg"}); err != nil {
		t.Fatal(err)
	}
	tgChat := addConversation(t, s, "tg", "tg-chat", "TG Chat")
	addMessages(t, s, domain.Message{ID: "tg-in", ConversationID: tgChat.ID, Text: "two", Created: 1})

	want := map[string]int{domain.ServiceWhatsApp: 1, domain.ServiceTelegram: 1}
	if got, err := s.UnreadByService(ctx); err != nil || !maps.Equal(got, want) {
		t.Errorf("UnreadByService = %v, %v; want %v", got, err, want)
	}

	if err := s.SetMuted(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}

	want = map[string]int{domain.ServiceTelegram: 1}
	if got, err := s.UnreadByService(ctx); err != nil || !maps.Equal(got, want) {
		t.Errorf("UnreadByService with a muted WhatsApp chat = %v, %v; want %v", got, err, want)
	}
}

// TestUpgradeConversationRemoteID_RenamesInPlace proves the simple case
// a connector hits once it resolves a better id for a conversation it
// already reported under a weaker one, such as Telegram learning a
// placeholder peer's real access hash: nothing yet exists under the
// new id, so the existing conversation just gets a new remote id,
// keeping its local id and messages.
func TestUpgradeConversationRemoteID_RenamesInPlace(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	placeholder, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:0", Title: "Telegram user", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	addMessages(t, s, domain.Message{ID: "m1", ConversationID: placeholder.ID, Text: "hi", Created: 1})

	upgraded, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890")
	if err != nil {
		t.Fatalf("UpgradeConversationRemoteID: %v", err)
	}

	if upgraded.ID != placeholder.ID || upgraded.RemoteID != "user:42:67890" {
		t.Errorf("upgraded = %+v, want id %q and remote id %q", upgraded, placeholder.ID, "user:42:67890")
	}

	if _, err := s.ConversationByRemote(ctx, "tg", "user:42:0"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ConversationByRemote(old id) = %v, want ErrNotFound", err)
	}

	messages, _, err := s.Messages(ctx, placeholder.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].ID != "m1" {
		t.Errorf("Messages after rename = %+v, %v; want the one message kept", messages, err)
	}
}

// TestUpgradeConversationRemoteID_MergesIntoTheExistingConversation
// proves the case this exists for: Telegram resolves a placeholder's
// real id to one a conversation already exists under - the two must
// never end up as duplicates. The placeholder's own message moves
// into the real conversation, which keeps its own id, and the
// placeholder is removed.
func TestUpgradeConversationRemoteID_MergesIntoTheExistingConversation(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	real, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:67890", Title: "Nadia", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(real): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "earlier", ConversationID: real.ID, Text: "hey", Created: 1})

	placeholder, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:0", Title: "Telegram user", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(placeholder): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "stray", ConversationID: placeholder.ID, Text: "hi", Created: 2})

	merged, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890")
	if err != nil {
		t.Fatalf("UpgradeConversationRemoteID: %v", err)
	}

	if merged.ID != real.ID || merged.RemoteID != "user:42:67890" {
		t.Errorf("merged = %+v, want the real conversation %q kept", merged, real.ID)
	}

	if _, err := s.Conversation(ctx, placeholder.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Conversation(placeholder) = %v, want ErrNotFound, placeholder must be removed", err)
	}

	messages, _, err := s.Messages(ctx, real.ID, "", 10)
	if err != nil || len(ids(messages)) != 2 || !slices.Contains(ids(messages), "earlier") || !slices.Contains(ids(messages), "stray") {
		t.Errorf("Messages after merge = %+v, %v; want both earlier and stray kept under %q", messages, err, real.ID)
	}
}

// TestUpgradeConversationRemoteID_DropsAMessageTheRealConversationAlreadyHas
// proves the merge never duplicates a message the real conversation
// already stored under the same remote id as one the placeholder also
// has - the placeholder's own copy is dropped, not kept as a second
// row.
func TestUpgradeConversationRemoteID_DropsAMessageTheRealConversationAlreadyHas(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	real, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:67890", Title: "Nadia", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(real): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "kept", ConversationID: real.ID, RemoteID: "9", Text: "hey", Created: 1})

	placeholder, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:0", Title: "Telegram user", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation(placeholder): %v", err)
	}
	addMessages(t, s, domain.Message{ID: "duplicate", ConversationID: placeholder.ID, RemoteID: "9", Text: "hey", Created: 1})

	if _, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890"); err != nil {
		t.Fatalf("UpgradeConversationRemoteID: %v", err)
	}

	messages, _, err := s.Messages(ctx, real.ID, "", 10)
	if err != nil || len(messages) != 1 || messages[0].ID != "kept" {
		t.Errorf("Messages after merge = %+v, %v; want only the real conversation's own message kept", messages, err)
	}
}

// TestUpgradeConversationRemoteID_ReportsNotFoundForAnUnknownOldID
// proves the common, harmless case: nothing is stored under oldRemoteID
// at all, such as a connector's own idempotent self-heal running
// against a peer that was never actually a placeholder. Nothing
// changes and the caller sees domain.ErrNotFound, the same as any
// other lookup by an id that does not exist.
func TestUpgradeConversationRemoteID_ReportsNotFoundForAnUnknownOldID(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	if _, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:0", "user:42:67890"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpgradeConversationRemoteID(unknown) = %v, want ErrNotFound", err)
	}
}

// TestUpgradeConversationRemoteID_SameIDIsANoOp proves that upgrading a
// conversation to the remote id it already has changes nothing and
// still returns it, rather than tripping over renaming an id onto
// itself.
func TestUpgradeConversationRemoteID_SameIDIsANoOp(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "tg")

	conv, _, err := s.EnsureConversation(ctx, domain.Conversation{
		AccountID: "tg", RemoteID: "user:42:67890", Title: "Nadia", Kind: domain.KindDirect,
	})
	if err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	got, err := s.UpgradeConversationRemoteID(ctx, "tg", "user:42:67890", "user:42:67890")
	if err != nil || got.ID != conv.ID {
		t.Errorf("UpgradeConversationRemoteID(same id) = %+v, %v; want %q unchanged", got, err, conv.ID)
	}
}
