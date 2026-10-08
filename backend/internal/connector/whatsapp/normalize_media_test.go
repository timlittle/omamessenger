package whatsapp

import (
	"encoding/base64"
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMedia_EachKindAndItsDownloadReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     *waE2E.Message
		want    *domain.Media
		wantRef mediaRef
		wantOK  bool
	}{
		{
			"link preview",
			&waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: strPtr("check this out https://example.com"), MatchedText: strPtr("https://example.com"),
				Title: strPtr("Example"), Description: strPtr("An example site"),
			}},
			&domain.Media{Kind: domain.MediaLink, URL: "https://example.com", Title: "Example", Description: "An example site"},
			mediaRef{},
			false,
		},
		{
			"link preview with neither title nor description",
			&waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: strPtr("hi")}},
			nil,
			mediaRef{},
			false,
		},
		{
			"photo",
			&waE2E.Message{ImageMessage: &waE2E.ImageMessage{
				Width: u32(800), Height: u32(600), FileLength: u64(1024), Mimetype: strPtr("image/jpeg"),
				DirectPath: strPtr("/v/photo"), MediaKey: []byte("key"), FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"),
			}},
			&domain.Media{Kind: domain.MediaPhoto, Width: 800, Height: 600, Size: 1024},
			mediaRef{Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"), FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), FileLength: 1024, Mimetype: "image/jpeg"},
			true,
		},
		{
			"video",
			&waE2E.Message{VideoMessage: &waE2E.VideoMessage{
				Width: u32(1280), Height: u32(720), Seconds: u32(30), FileLength: u64(2048), Mimetype: strPtr("video/mp4"),
				DirectPath: strPtr("/v/video"), MediaKey: []byte("key"),
			}},
			&domain.Media{Kind: domain.MediaVideo, Width: 1280, Height: 720, Duration: 30, Size: 2048},
			mediaRef{Kind: mediaKindVideo, DirectPath: "/v/video", MediaKey: []byte("key"), FileLength: 2048, Mimetype: "video/mp4"},
			true,
		},
		{
			"voice note",
			&waE2E.Message{AudioMessage: &waE2E.AudioMessage{
				PTT: boolPtr(true), Seconds: u32(12), FileLength: u64(512), Mimetype: strPtr("audio/ogg"),
				DirectPath: strPtr("/v/voice"), MediaKey: []byte("key"),
			}},
			&domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg", Duration: 12, Size: 512},
			mediaRef{Kind: mediaKindAudio, DirectPath: "/v/voice", MediaKey: []byte("key"), FileLength: 512, Mimetype: "audio/ogg"},
			true,
		},
		{
			"ordinary audio",
			&waE2E.Message{AudioMessage: &waE2E.AudioMessage{FileLength: u64(512)}},
			&domain.Media{Kind: domain.MediaFile, FileName: "audio-message.ogg", Size: 512},
			mediaRef{Kind: mediaKindAudio, FileLength: 512},
			true,
		},
		{
			"document with a name",
			&waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
				FileName: strPtr("report.pdf"), FileLength: u64(4096), Mimetype: strPtr("application/pdf"),
				DirectPath: strPtr("/v/doc"), MediaKey: []byte("key"),
			}},
			&domain.Media{Kind: domain.MediaFile, FileName: "report.pdf", Size: 4096},
			mediaRef{Kind: mediaKindDocument, DirectPath: "/v/doc", MediaKey: []byte("key"), FileLength: 4096, Mimetype: "application/pdf"},
			true,
		},
		{
			"document with no name",
			&waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{FileLength: u64(100)}},
			&domain.Media{Kind: domain.MediaFile, FileName: "file", Size: 100},
			mediaRef{Kind: mediaKindDocument, FileLength: 100},
			true,
		},
		{
			"sticker",
			&waE2E.Message{StickerMessage: &waE2E.StickerMessage{
				Width: u32(512), Height: u32(512), FileLength: u64(2048), Mimetype: strPtr("image/webp"), Emojis: strPtr("😀"),
				DirectPath: strPtr("/v/sticker"), MediaKey: []byte("key"),
			}},
			&domain.Media{Kind: domain.MediaSticker, Width: 512, Height: 512, Size: 2048, FileName: "sticker.webp", Emoji: "😀"},
			mediaRef{Kind: mediaKindSticker, DirectPath: "/v/sticker", MediaKey: []byte("key"), FileLength: 2048, Mimetype: "image/webp"},
			true,
		},
		{
			"plain text carries no media",
			&waE2E.Message{Conversation: strPtr("hi")},
			nil,
			mediaRef{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotMedia, gotRef, gotOK := mediaAndRef(tt.msg)
			if !reflect.DeepEqual(gotMedia, tt.want) {
				t.Errorf("media = %+v, want %+v", gotMedia, tt.want)
			}

			if !reflect.DeepEqual(gotRef, tt.wantRef) || gotOK != tt.wantOK {
				t.Errorf("downloadRef = %+v, %t; want %+v, %t", gotRef, gotOK, tt.wantRef, tt.wantOK)
			}

			if got := media(tt.msg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("media() = %+v, want %+v", got, tt.want)
			}

			ref, ok := downloadRef(tt.msg)
			if !reflect.DeepEqual(ref, tt.wantRef) || ok != tt.wantOK {
				t.Errorf("downloadRef() = %+v, %t; want %+v, %t", ref, ok, tt.wantRef, tt.wantOK)
			}
		})
	}
}

func TestThumb_EncodesOrReportsNone(t *testing.T) {
	t.Parallel()

	if got := thumb(nil); got != "" {
		t.Errorf("thumb(nil) = %q, want empty", got)
	}

	jpeg := []byte{0xFF, 0xD8, 0xFF}
	if got := thumb(jpeg); got != base64.StdEncoding.EncodeToString(jpeg) {
		t.Errorf("thumb(%v) = %q, want the base64 encoding", jpeg, got)
	}
}

func TestAppInfo_MapsEachKindToItsWhatsmeowType(t *testing.T) {
	t.Parallel()

	tests := map[mediaKind]whatsmeow.MediaType{
		mediaKindImage:     whatsmeow.MediaImage,
		mediaKindVideo:     whatsmeow.MediaVideo,
		mediaKindAudio:     whatsmeow.MediaAudio,
		mediaKindDocument:  whatsmeow.MediaDocument,
		mediaKind("bogus"): whatsmeow.MediaDocument,
	}

	for kind, want := range tests {
		if got := appInfo(kind); got != want {
			t.Errorf("appInfo(%q) = %q, want %q", kind, got, want)
		}
	}
}

// FuzzMedia checks that media and downloadRef never panic on a message
// decoded from arbitrary bytes.
func FuzzMedia(f *testing.F) {
	seed, _ := proto.Marshal(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{FileLength: u64(10)}})
	f.Add(seed)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		var msg waE2E.Message
		if err := proto.Unmarshal(data, &msg); err != nil {
			return
		}

		media(&msg)
		downloadRef(&msg)
	})
}
