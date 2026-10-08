package whatsapp

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// captureLog redirects the standard logger to a buffer for the rest of
// t, restoring stderr when it ends. It is not run in parallel with
// anything else in this package that logs, so nothing else can write
// into the buffer while it is captured.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	return &buf
}

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
		{"interactive response", &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{}}},
		{"buttons", &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{}}},
		{"buttons response", &waE2E.Message{ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{}}},
		{"template", &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{}}},
		{"template button reply", &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{}}},
		{"list", &waE2E.Message{ListMessage: &waE2E.ListMessage{}}},
		{"list response", &waE2E.Message{ListResponseMessage: &waE2E.ListResponseMessage{}}},
		{"order", &waE2E.Message{OrderMessage: &waE2E.OrderMessage{}}},
		{"product", &waE2E.Message{ProductMessage: &waE2E.ProductMessage{}}},
		{"highly structured", &waE2E.Message{HighlyStructuredMessage: &waE2E.HighlyStructuredMessage{}}},
		{"bot invoke", &waE2E.Message{BotInvokeMessage: &waE2E.FutureProofMessage{}}},
		{"document with caption", &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{}}},
		{"lottie sticker", &waE2E.Message{LottieStickerMessage: &waE2E.FutureProofMessage{}}},
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

	// InvoiceMessage is a real WhatsApp Business content kind this
	// connector has never been taught to show.
	field, ok := unknownContentKind(&waE2E.Message{InvoiceMessage: &waE2E.InvoiceMessage{}})
	if !ok {
		t.Fatal("unknownContentKind = false, want true for a kind this connector does not recognise")
	}
	if field != "invoiceMessage" {
		t.Errorf("unknownContentKind field = %q, want %q", field, "invoiceMessage")
	}
}

// TestPlaceholderFields_NamesPopulatedFieldsUpToTheDepthLimit confirms
// the diagnostic messageText logs for a still-generic placeholder
// names only populated fields, never a value, and expands a nested
// message into a dotted path only up to fieldPathDepth deep, so an
// unrecognised business kind's shape can be told apart without the
// line growing unbounded.
func TestPlaceholderFields_NamesPopulatedFieldsUpToTheDepthLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{"nothing populated", &waE2E.Message{}, ""},
		{"one flat field", &waE2E.Message{InvoiceMessage: &waE2E.InvoiceMessage{}}, "invoiceMessage"},
		{
			"a nested field one level deep",
			&waE2E.Message{OrderMessage: &waE2E.OrderMessage{OrderTitle: strPtr("Order #1")}},
			"orderMessage.orderTitle",
		},
		{
			"a message field with nothing populated inside it stops at its own name",
			&waE2E.Message{OrderMessage: &waE2E.OrderMessage{}},
			"orderMessage",
		},
		{
			"depth stops expanding a path past three levels",
			&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
				InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
					Cards: []*waE2E.InteractiveMessage{{Body: &waE2E.InteractiveMessage_Body{Text: strPtr("hi")}}},
				}},
			}},
			// cards is a repeated field, so it is reported by name alone
			// rather than descended into, even though a fourth level
			// (its card's own body text) would otherwise be there.
			"interactiveMessage.carouselMessage.cards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := fieldPaths(tt.msg); got != tt.want {
				t.Errorf("fieldPaths(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestLogContentlessDrop_NamesWhatsAppsSystemChatSeparately confirms a
// contentless message in WhatsApp's own "0" system chat logs its own
// diagnostic line, naming the field it carried, rather than an
// ordinary dropped-message line indistinguishable from any other
// chat's; this is not run with t.Parallel so capturing the log output
// stays reliable.
func TestLogContentlessDrop_NamesWhatsAppsSystemChatSeparately(t *testing.T) {
	buf := captureLog(t)

	logContentlessDrop(types.PSAJID, &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}})

	if got := buf.String(); !strings.Contains(got, "whatsapp: system chat message dropped (reason=contentless, fields=protocolMessage)") {
		t.Errorf("log output = %q, want the system chat's own diagnostic line naming the populated field", got)
	}
}

// TestLogContentlessDrop_IsAnOrdinaryDropForAnyoneElse confirms an
// everyday chat's contentless drop still logs the plain line, with no
// mention of the system chat at all.
func TestLogContentlessDrop_IsAnOrdinaryDropForAnyoneElse(t *testing.T) {
	buf := captureLog(t)

	logContentlessDrop(types.NewJID("15551234567", types.DefaultUserServer), &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}})

	got := buf.String()
	if !strings.Contains(got, "whatsapp: dropped message (reason=contentless)") {
		t.Errorf("log output = %q, want the ordinary dropped-message line", got)
	}
	if strings.Contains(got, "system chat") {
		t.Errorf("log output = %q, want no mention of the system chat for an ordinary contact", got)
	}
}

// TestHandleHistorySync_LogsTheSystemChatsOwnReasonWhenDropped confirms
// that if WhatsApp's own "0" system account's history sync ever
// carries nothing but contentless messages (one hypothesis for the
// conversation never appearing at all), this connector logs its own
// diagnostic line for it, rather than the plain line any other empty
// chat gets; this is not run with t.Parallel so capturing the log
// output stays reliable.
func TestHandleHistorySync_LogsTheSystemChatsOwnReasonWhenDropped(t *testing.T) {
	buf := captureLog(t)

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev, media := newFakeDevice(), newTestMediaStore(t)
	var sink connectortest.Sink

	notice := &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: strPtr("H1")},
		Message:          &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}},
		MessageTimestamp: u64(1),
	}}
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{ID: strPtr("0@s.whatsapp.net"), Messages: []*waHistorySync.HistorySyncMsg{notice}}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported with no real message", sink.Lines())
	}
	if got := buf.String(); !strings.Contains(got, "whatsapp: system chat message dropped (reason=no-real-content, fields=protocolMessage)") {
		t.Errorf("log output = %q, want the system chat's own diagnostic line", got)
	}
}
