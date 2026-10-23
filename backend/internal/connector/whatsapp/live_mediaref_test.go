package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

// TestHandleMessage_SavesTheMediaReferenceOfAWrappedMessage covers media
// that arrives wrapped, as an album's photo or video, or in a
// disappearing-message chat, always does: its download reference must be
// saved from the message inside the wrapper, the same one its kind and
// text come from, or the media can never be fetched.
func TestHandleMessage_SavesTheMediaReferenceOfAWrappedMessage(t *testing.T) {
	t.Parallel()

	video := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		DirectPath: strPtr("/v/video"), MediaKey: []byte("key"), FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"),
		Mimetype: strPtr("video/mp4"), FileLength: u64(2048),
	}}
	wrappers := map[string]*waE2E.Message{
		"album child":  {AssociatedChildMessage: &waE2E.FutureProofMessage{Message: video}},
		"disappearing": {EphemeralMessage: &waE2E.FutureProofMessage{Message: video}},
	}

	for name, wrapped := range wrappers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, dev, sink, media := handlerMediaFixture(t)
			c.handleMessage(t.Context(), sink, dev, media, &events.Message{Info: liveInfo(), Message: wrapped})

			ref, ok, err := media.get(t.Context(), "15551234567@s.whatsapp.net", "M1")
			if err != nil || !ok || ref.DirectPath != "/v/video" {
				t.Fatalf("saved media reference = %+v, %v, %v; want the wrapped video's own", ref, ok, err)
			}
		})
	}
}
