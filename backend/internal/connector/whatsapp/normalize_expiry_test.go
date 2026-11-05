package whatsapp

// normalize_expiry_test.go checks message's own handling of
// disappearing and view-once messages, split out of
// normalize_message_test.go to keep that file within this codebase's
// size guideline.

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// TestMessage_ViewOnceIsNeverDownloaded checks that a view-once photo,
// video or voice note, however WhatsApp wrapped it, is reported with
// no media at all and a placeholder naming what it was, whether the
// wrapper survived (a history sync) or whatsmeow already peeled it and
// left only the inner media's own flag (a live message).
func TestMessage_ViewOnceIsNeverDownloaded(t *testing.T) {
	t.Parallel()

	photo := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: strPtr("look")}}
	video := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: strPtr("look")}}
	voice := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: boolPtr(true)}}

	tests := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{"wrapped photo, history sync", &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: photo}}, "View-once photo: open it on your phone"},
		{"wrapped v2 video", &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: video}}, "View-once video: open it on your phone"},
		{"wrapped v2 extension voice", &waE2E.Message{ViewOnceMessageV2Extension: &waE2E.FutureProofMessage{Message: voice}}, "View-once voice note: open it on your phone"},
		{"nested in an ephemeral wrapper", &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
			ViewOnceMessage: &waE2E.FutureProofMessage{Message: photo},
		}}}, "View-once photo: open it on your phone"},
		{"unwrapped photo, own flag set (a live message)", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: strPtr("look"), ViewOnce: boolPtr(true)}}, "View-once photo: open it on your phone"},
		{"unwrapped video, own flag set", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: strPtr("look"), ViewOnce: boolPtr(true)}}, "View-once video: open it on your phone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := message(t.Context(), newFakeDevice(), testInfo(), tt.msg)
			if !got.ViewOnce || got.Media != nil || got.Text != tt.want {
				t.Errorf("message(%s) = ViewOnce=%t Media=%+v Text=%q, want ViewOnce, no media, %q",
					tt.name, got.ViewOnce, got.Media, got.Text, tt.want)
			}
		})
	}
}

// TestMessage_ReportsADisappearingPeriod checks that a disappearing
// message's own period, from ContextInfo.Expiration, is carried for
// the sweeper to act on, and that an ordinary message carries none.
func TestMessage_ReportsADisappearingPeriod(t *testing.T) {
	t.Parallel()

	disappearing := message(t.Context(), newFakeDevice(), testInfo(), &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: strPtr("hi"), ContextInfo: &waE2E.ContextInfo{Expiration: uint32Ptr(86400)}},
	})
	if disappearing.ExpireAfter != 86400 {
		t.Errorf("message.ExpireAfter = %d, want 86400", disappearing.ExpireAfter)
	}

	plain := message(t.Context(), newFakeDevice(), testInfo(), &waE2E.Message{Conversation: strPtr("hi")})
	if plain.ExpireAfter != 0 {
		t.Errorf("message.ExpireAfter = %d, want 0 for a message with no disappearing timer", plain.ExpireAfter)
	}
}
