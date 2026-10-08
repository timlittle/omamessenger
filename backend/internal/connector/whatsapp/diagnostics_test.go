package whatsapp

import (
	"errors"
	"net/http"
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
// beyond the four safe categories, and that the statuses WhatsApp's CDN
// answers with for a stale link (403, 404 and 410) are all told apart
// from any other HTTP status it might answer with: retry.go only knows
// how to recover from the former.
func TestDownloadFailureClass_SortsEveryDownloadErrorIntoItsSafeCategory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want string
	}{
		"a worn-out hash":       {whatsmeow.ErrInvalidMediaHMAC, "decrypt"},
		"an expired link (403)": {whatsmeow.ErrMediaDownloadFailedWith403, "expired"},
		"an expired link (404)": {whatsmeow.ErrMediaDownloadFailedWith404, "expired"},
		"an expired link (410)": {whatsmeow.ErrMediaDownloadFailedWith410, "expired"},
		"another HTTP status":   {whatsmeow.DownloadHTTPError{Response: &http.Response{StatusCode: 500}}, "http"},
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

// TestDownloadFailureStatus_ReadsTheNumericStatusFromAnHTTPFailure
// confirms the status logDownloadFailed is safe to log alongside the
// class comes straight from whatsmeow's own DownloadHTTPError, and that
// a failure with no HTTP status at all (a decrypt mismatch or a plain
// network error) reports none.
func TestDownloadFailureStatus_ReadsTheNumericStatusFromAnHTTPFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err        error
		wantStatus int
		wantOK     bool
	}{
		"an expired link (403)": {whatsmeow.ErrMediaDownloadFailedWith403, 403, true},
		"an expired link (410)": {whatsmeow.ErrMediaDownloadFailedWith410, 410, true},
		"another HTTP status":   {whatsmeow.DownloadHTTPError{Response: &http.Response{StatusCode: 500}}, 500, true},
		"a worn-out hash":       {whatsmeow.ErrInvalidMediaHMAC, 0, false},
		"a plain network error": {errors.New("dial tcp: connection refused"), 0, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status, ok := downloadFailureStatus(tt.err)
			if status != tt.wantStatus || ok != tt.wantOK {
				t.Errorf("downloadFailureStatus(%v) = %d, %v, want %d, %v", tt.err, status, ok, tt.wantStatus, tt.wantOK)
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
