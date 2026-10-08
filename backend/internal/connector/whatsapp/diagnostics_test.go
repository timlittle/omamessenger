package whatsapp

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
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
		{"album header", &waE2E.Message{AlbumMessage: &waE2E.AlbumMessage{}}},
		{"album child", &waE2E.Message{AssociatedChildMessage: &waE2E.FutureProofMessage{}}},
		{"interactive", &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{}}},
		{"buttons", &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{}}},
		{"template", &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{}}},
		{"template button reply", &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{}}},
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

// TestDownloadFailureClass_SortsEveryDownloadErrorIntoItsSafeCategory
// confirms the class logDownloadFailed reports never reflects anything
// beyond the four safe categories, and that WhatsApp's own 404/410 are
// told apart from any other HTTP status it might answer with.
func TestDownloadFailureClass_SortsEveryDownloadErrorIntoItsSafeCategory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want string
	}{
		"a worn-out hash":       {whatsmeow.ErrInvalidMediaHMAC, "decrypt"},
		"an expired link (404)": {whatsmeow.ErrMediaDownloadFailedWith404, "expired"},
		"an expired link (410)": {whatsmeow.ErrMediaDownloadFailedWith410, "expired"},
		"another HTTP status":   {whatsmeow.ErrMediaDownloadFailedWith403, "http"},
		"a plain network error": {errors.New("dial tcp: connection refused"), "network"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := downloadFailureClass(tt.err); got != tt.want {
				t.Errorf("downloadFailureClass(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestUnknownContentKind_NamesAFieldThisConnectorDoesNotHandle(t *testing.T) {
	t.Parallel()

	// ProductMessage is a real WhatsApp Business content kind this
	// connector has never been taught to show.
	field, ok := unknownContentKind(&waE2E.Message{ProductMessage: &waE2E.ProductMessage{}})
	if !ok {
		t.Fatal("unknownContentKind = false, want true for a kind this connector does not recognise")
	}
	if field != "productMessage" {
		t.Errorf("unknownContentKind field = %q, want %q", field, "productMessage")
	}
}
