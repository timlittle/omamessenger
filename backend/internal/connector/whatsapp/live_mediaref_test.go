package whatsapp

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
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

// TestHandleMessage_KeepsTheFirstSendersMediaRefWhenAnotherOneReusesTheID
// covers the same id-reuse fix as keys.go's putMessageKey, for media_refs:
// Mallory naming Alice's already-saved message id in her own message
// must never let her video become the one fetched when the user opens
// what still shows as Alice's message.
func TestHandleMessage_KeepsTheFirstSendersMediaRefWhenAnotherOneReusesTheID(t *testing.T) {
	t.Parallel()

	c, dev, sink, media := handlerMediaFixture(t)
	chat := types.NewJID("15551234567", types.DefaultUserServer)
	alice := types.NewJID("15551111111", types.DefaultUserServer)
	mallory := types.NewJID("15559876543", types.DefaultUserServer)

	aliceVideo := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		DirectPath: strPtr("/v/alice"), MediaKey: []byte("a-key"), FileSHA256: []byte("a-sha"), FileEncSHA256: []byte("a-enc"),
		Mimetype: strPtr("video/mp4"), FileLength: u64(1024),
	}}
	malloryVideo := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		DirectPath: strPtr("/v/mallory"), MediaKey: []byte("m-key"), FileSHA256: []byte("m-sha"), FileEncSHA256: []byte("m-enc"),
		Mimetype: strPtr("video/mp4"), FileLength: u64(2048),
	}}

	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: alice}, ID: "M0", Timestamp: time.Unix(1, 0)},
		Message: aliceVideo,
	})
	c.handleMessage(t.Context(), sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: mallory}, ID: "M0", Timestamp: time.Unix(2, 0)},
		Message: malloryVideo,
	})

	ref, ok, err := media.get(t.Context(), remoteID(chat), "M0")
	if err != nil || !ok || ref.DirectPath != "/v/alice" {
		t.Errorf("saved media reference = %+v, %v, %v; want Alice's own, not overwritten by the id reused by Mallory", ref, ok, err)
	}
}
