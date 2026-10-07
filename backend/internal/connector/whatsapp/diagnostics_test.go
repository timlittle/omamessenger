package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestUnknownContentKind_RecognisesEveryKindThisConnectorHandles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *waE2E.Message
	}{
		{"plain text", &waE2E.Message{Conversation: strPtr("hi")}},
		{"extended text", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: strPtr("hi")}}},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}},
		{"poll", &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{}}},
		{"poll v5", &waE2E.Message{PollCreationMessageV5: &waE2E.PollCreationMessage{}}},
		{"group invite", &waE2E.Message{GroupInviteMessage: &waE2E.GroupInviteMessage{}}},
		{"sticker pack", &waE2E.Message{StickerPackMessage: &waE2E.StickerPackMessage{}}},
		{"reaction", &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{}}},
		{"protocol", &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}},
		{"call", &waE2E.Message{Call: &waE2E.Call{}}},
		{"nothing at all", &waE2E.Message{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if field, ok := unknownContentKind(tt.msg); ok {
				t.Errorf("unknownContentKind(%s) = %q, true; want it recognised", tt.name, field)
			}
		})
	}
}

func TestUnknownContentKind_NamesAFieldThisConnectorDoesNotHandle(t *testing.T) {
	t.Parallel()

	// TemplateMessage is a real WhatsApp Business content kind this
	// connector has never been taught to show.
	field, ok := unknownContentKind(&waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{}})
	if !ok {
		t.Fatal("unknownContentKind = false, want true for a kind this connector does not recognise")
	}
	if field != "templateMessage" {
		t.Errorf("unknownContentKind field = %q, want %q", field, "templateMessage")
	}
}
