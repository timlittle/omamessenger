package whatsapp

import (
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestIsReaction_AndReaction_ReadTheTargetAndEmoji(t *testing.T) {
	t.Parallel()

	msg := &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
		Key: &waCommon.MessageKey{ID: strPtr("M1")}, Text: strPtr("👍"),
	}}

	if !isReaction(msg) {
		t.Fatal("isReaction(a reaction) = false, want true")
	}

	id, emoji := reaction(msg)
	if id != "M1" || emoji != "👍" {
		t.Errorf("reaction = %q, %q; want M1, 👍", id, emoji)
	}

	if isReaction(&waE2E.Message{Conversation: strPtr("hi")}) {
		t.Error("isReaction(plain text) = true, want false")
	}
}

func TestReaction_EmptyTextClearsIt(t *testing.T) {
	t.Parallel()

	msg := &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Key: &waCommon.MessageKey{ID: strPtr("M1")}}}
	if _, emoji := reaction(msg); emoji != "" {
		t.Errorf("reaction emoji = %q, want empty for a cleared reaction", emoji)
	}
}

func TestIsRevoke_AndRevoke_ReadTheDeletedMessage(t *testing.T) {
	t.Parallel()

	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M2")},
	}}

	if !isRevoke(msg) {
		t.Fatal("isRevoke(a revoke) = false, want true")
	}

	if got := revoke(msg); got != "M2" {
		t.Errorf("revoke = %q, want M2", got)
	}

	if isRevoke(&waE2E.Message{Conversation: strPtr("hi")}) {
		t.Error("isRevoke(plain text) = true, want false")
	}
}

func TestIsEdit_AndEdit_ReplaceTheOriginalMessage(t *testing.T) {
	t.Parallel()

	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: strPtr("M3")},
		EditedMessage: &waE2E.Message{Conversation: strPtr("corrected")},
	}}

	if !isEdit(msg) {
		t.Fatal("isEdit(an edit) = false, want true")
	}

	got := edit(t.Context(), newFakeDevice(), testInfo(), msg)
	want := domain.Message{
		RemoteID: "M3", SenderID: "15551234567@s.whatsapp.net", SenderName: "Nadia Rahman",
		Text: "corrected", Status: domain.StatusReceived, Created: 1_800_000_000_000, Edited: true,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("edit = %+v\nwant  %+v", got, want)
	}

	if isEdit(&waE2E.Message{Conversation: strPtr("hi")}) {
		t.Error("isEdit(plain text) = true, want false")
	}
}

func TestReceiptStatus_MapsKnownTypesAndIgnoresTheRest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		receiptType types.ReceiptType
		want        string
	}{
		{types.ReceiptTypeDelivered, domain.StatusDelivered},
		{types.ReceiptTypeSender, domain.StatusDelivered},
		{types.ReceiptTypeRead, domain.StatusRead},
		{types.ReceiptTypeReadSelf, domain.StatusRead},
		{types.ReceiptTypeRetry, ""},
		{types.ReceiptTypePlayed, ""},
	}

	for _, tt := range tests {
		if got := receiptStatus(tt.receiptType); got != tt.want {
			t.Errorf("receiptStatus(%v) = %q, want %q", tt.receiptType, got, tt.want)
		}
	}
}

func TestReactionTally_GroupsCountsAndFindsOurOwn(t *testing.T) {
	t.Parallel()

	tally := reactionTally(map[string]string{
		"a@s.whatsapp.net": "👍",
		"b@s.whatsapp.net": "👍",
		"self":             "❤️",
	}, "self")

	want := []domain.Reaction{
		{Emoji: "❤️", Count: 1, Mine: true},
		{Emoji: "👍", Count: 2, Mine: false},
	}
	if !reflect.DeepEqual(tally, want) {
		t.Errorf("reactionTally = %+v, want %+v", tally, want)
	}
}

func TestReactionTally_EmptyWhenNobodyReacted(t *testing.T) {
	t.Parallel()

	if got := reactionTally(map[string]string{}, "self"); len(got) != 0 {
		t.Errorf("reactionTally(none) = %+v, want empty", got)
	}
}

func TestTypingActive_ComposingIsActivePausedIsNot(t *testing.T) {
	t.Parallel()

	if !typingActive(types.ChatPresenceComposing) {
		t.Error("typingActive(composing) = false, want true")
	}

	if typingActive(types.ChatPresencePaused) {
		t.Error("typingActive(paused) = true, want false")
	}
}

// FuzzReactionRevokeEdit checks that the reaction, revoke and edit
// readers never panic on a message decoded from arbitrary bytes,
// whatever protocol message type or key it claims to have.
func FuzzReactionRevokeEdit(f *testing.F) {
	seed, _ := proto.Marshal(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: strPtr("M")},
		EditedMessage: &waE2E.Message{Conversation: strPtr("x")},
	}})
	f.Add(seed)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		var msg waE2E.Message
		if err := proto.Unmarshal(data, &msg); err != nil {
			return
		}

		if isReaction(&msg) {
			reaction(&msg)
		}

		if isRevoke(&msg) {
			revoke(&msg)
		}

		if isEdit(&msg) {
			edit(t.Context(), newFakeDevice(), testInfo(), &msg)
		}
	})
}
