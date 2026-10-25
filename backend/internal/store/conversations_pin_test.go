package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSetPinned_PinsAndUnpins(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if err := s.SetPinned(ctx, "chat", true); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Pinned {
		t.Fatalf("Conversation after pin = %+v, %v; want Pinned true", c, err)
	}

	if err := s.SetPinned(ctx, "chat", false); err != nil {
		t.Fatal(err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || c.Pinned {
		t.Fatalf("Conversation after unpin = %+v, %v; want Pinned false", c, err)
	}

	if err := s.SetPinned(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetPinned(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetPinned_AssignsOrderToTheBottomOfThePinnedGroup(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	first := addConversation(t, s, "wa", "first", "First")
	second := addConversation(t, s, "wa", "second", "Second")
	third := addConversation(t, s, "wa", "third", "Third")

	for _, id := range []string{first.ID, second.ID, third.ID} {
		if err := s.SetPinned(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}

	// Each pin lands at the bottom of the pinned group, in the order it
	// was made, not by the conversations' own unrelated last-activity.
	want := []string{"first", "second", "third"}
	if !slices.Equal(gotIDs, want) {
		t.Errorf("Conversations order = %v, want %v", gotIDs, want)
	}
}

func TestSetPinned_RepinningLeavesItsOrderAlone(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	first := addConversation(t, s, "wa", "first", "First")
	second := addConversation(t, s, "wa", "second", "Second")

	for _, id := range []string{first.ID, second.ID} {
		if err := s.SetPinned(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}

	// Pinning an already-pinned conversation again must not bump it to
	// the back of the group.
	if err := s.SetPinned(ctx, first.ID, true); err != nil {
		t.Fatal(err)
	}

	got, err := s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}

	if want := []string{"first", "second"}; !slices.Equal(gotIDs, want) {
		t.Errorf("Conversations order after repinning = %v, want %v", gotIDs, want)
	}
}

func TestSetPinned_UnpinningClearsOrder(t *testing.T) {
	t.Parallel()

	s, chat := openChatStore(t)
	ctx := t.Context()

	if err := s.SetPinned(ctx, chat.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPinned(ctx, chat.ID, false); err != nil {
		t.Fatal(err)
	}

	got, err := s.Conversation(ctx, chat.ID)
	if err != nil || got.PinOrder != 0 {
		t.Errorf("Conversation after unpin = %+v, %v; want PinOrder 0", got, err)
	}
}

func TestMovePin_SwapsAdjacentPinnedConversations(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	first := addConversation(t, s, "wa", "first", "First")
	second := addConversation(t, s, "wa", "second", "Second")
	third := addConversation(t, s, "wa", "third", "Third")

	for _, id := range []string{first.ID, second.ID, third.ID} {
		if err := s.SetPinned(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}

	// first, second, third -> second, first, third
	neighbor, err := s.MovePin(ctx, "second", true)
	if err != nil || neighbor != "first" {
		t.Fatalf("MovePin(second, up) = %q, %v; want first, nil", neighbor, err)
	}

	got, err := s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}
	if want := []string{"second", "first", "third"}; !slices.Equal(gotIDs, want) {
		t.Fatalf("order after moving second up = %v, want %v", gotIDs, want)
	}

	// second, first, third -> second, third, first
	neighbor, err = s.MovePin(ctx, "first", false)
	if err != nil || neighbor != "third" {
		t.Fatalf("MovePin(first, down) = %q, %v; want third, nil", neighbor, err)
	}

	got, err = s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	gotIDs = nil
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}
	if want := []string{"second", "third", "first"}; !slices.Equal(gotIDs, want) {
		t.Errorf("order after moving first down = %v, want %v", gotIDs, want)
	}
}

func TestMovePin_NoopAtEitherEdge(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	first := addConversation(t, s, "wa", "first", "First")
	second := addConversation(t, s, "wa", "second", "Second")

	for _, id := range []string{first.ID, second.ID} {
		if err := s.SetPinned(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}

	if neighbor, err := s.MovePin(ctx, "first", true); err != nil || neighbor != "" {
		t.Errorf("MovePin(first, up) at the top = %q, %v; want \"\", nil", neighbor, err)
	}
	if neighbor, err := s.MovePin(ctx, "second", false); err != nil || neighbor != "" {
		t.Errorf("MovePin(second, down) at the bottom = %q, %v; want \"\", nil", neighbor, err)
	}

	got, err := s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}
	if want := []string{"first", "second"}; !slices.Equal(gotIDs, want) {
		t.Errorf("order after no-op moves = %v, want %v", gotIDs, want)
	}
}

func TestMovePin_RefusesAConversationThatIsNotPinned(t *testing.T) {
	t.Parallel()

	s, chat := openChatStore(t)
	ctx := t.Context()

	if _, err := s.MovePin(ctx, chat.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MovePin(not pinned) = %v, want ErrNotFound", err)
	}

	if _, err := s.MovePin(ctx, "missing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("MovePin(missing) = %v, want ErrNotFound", err)
	}
}

func TestConversations_OrdersPinnedFirst(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	addConversation(t, s, "wa", "old-pinned", "Old Pinned")
	addConversation(t, s, "wa", "new", "New")
	addConversation(t, s, "wa", "new-pinned", "New Pinned")
	addMessages(t, s,
		domain.Message{ID: "m1", ConversationID: "old-pinned", Text: "one", Created: 10},
		domain.Message{ID: "m2", ConversationID: "new", Text: "two", Created: 20},
		domain.Message{ID: "m3", ConversationID: "new-pinned", Text: "three", Created: 30},
	)

	if err := s.SetPinned(ctx, "old-pinned", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPinned(ctx, "new-pinned", true); err != nil {
		t.Fatal(err)
	}

	// "old-pinned" has the oldest activity of the three but still leads
	// "new", which is not pinned, because pinned conversations always
	// come first; among the pinned ones, the order they were pinned in
	// leads (see TestSetPinned_AssignsOrderToTheBottomOfThePinnedGroup),
	// not their own last activity; among the unpinned ones, the newest
	// activity leads.
	got, err := s.Conversations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var gotIDs []string
	for _, c := range got {
		gotIDs = append(gotIDs, c.ID)
	}

	want := []string{"old-pinned", "new-pinned", "new"}
	if !slices.Equal(gotIDs, want) {
		t.Errorf("Conversations order = %v, want %v", gotIDs, want)
	}
}

func TestSetOrganized_SetsBothOrReportsNoChange(t *testing.T) {
	t.Parallel()

	s, _ := openChatStore(t)
	ctx := t.Context()

	if changed, err := s.SetOrganized(ctx, "chat", true, true); err != nil || !changed {
		t.Fatalf("SetOrganized(true, true) = %t, %v; want a change", changed, err)
	}

	if c, err := s.Conversation(ctx, "chat"); err != nil || !c.Pinned || !c.Archived {
		t.Fatalf("Conversation after SetOrganized = %+v, %v; want both set", c, err)
	}

	if changed, err := s.SetOrganized(ctx, "chat", true, true); err != nil || changed {
		t.Errorf("SetOrganized with the same values = %t, %v; want no change", changed, err)
	}

	if changed, err := s.SetOrganized(ctx, "chat", false, false); err != nil || !changed {
		t.Errorf("SetOrganized(false, false) = %t, %v; want a change", changed, err)
	}

	if _, err := s.SetOrganized(ctx, "missing", true, true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetOrganized(missing) = %v, want ErrNotFound", err)
	}
}

func TestSetOrganized_AssignsOrderOnlyWhenNewlyPinned(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := t.Context()
	addAccount(t, s, "wa")
	already := addConversation(t, s, "wa", "already", "Already Pinned")
	fresh := addConversation(t, s, "wa", "fresh", "Freshly Pinned")

	if err := s.SetPinned(ctx, already.ID, true); err != nil {
		t.Fatal(err)
	}

	before, err := s.Conversation(ctx, already.ID)
	if err != nil {
		t.Fatal(err)
	}

	// A sync reporting a conversation already pinned here must leave its
	// place alone, even though it is also the newest activity, which
	// would otherwise tempt a naive implementation into reassigning it.
	if _, err := s.SetOrganized(ctx, already.ID, true, false); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Conversation(ctx, already.ID); err != nil || got.PinOrder != before.PinOrder {
		t.Errorf("Conversation after a no-change sync = %+v, %v; want PinOrder %d unchanged", got, err, before.PinOrder)
	}

	// A sync reporting a conversation pinned for the first time joins
	// the bottom of the pinned group, same as SetPinned.
	if _, err := s.SetOrganized(ctx, fresh.ID, true, false); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Conversation(ctx, fresh.ID); err != nil || got.PinOrder <= before.PinOrder {
		t.Errorf("Conversation newly pinned by sync = %+v, %v; want PinOrder past %d", got, err, before.PinOrder)
	}
}

func TestSetOrganized_UnpinningClearsOrder(t *testing.T) {
	t.Parallel()

	s, chat := openChatStore(t)
	ctx := t.Context()

	if _, err := s.SetOrganized(ctx, chat.ID, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetOrganized(ctx, chat.ID, false, false); err != nil {
		t.Fatal(err)
	}

	if got, err := s.Conversation(ctx, chat.ID); err != nil || got.PinOrder != 0 {
		t.Errorf("Conversation after an unpinning sync = %+v, %v; want PinOrder 0", got, err)
	}
}
