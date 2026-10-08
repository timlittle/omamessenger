package telegram

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// stripped is a stripped thumbnail as Telegram sends it: a version byte,
// the height and width, then the JPEG body without its standard header.
// It returns a fresh value each time: gotd's encoder mutates a struct's
// flags fields while encoding, so sharing one pointer across parallel
// tests would race.
func stripped() *tg.PhotoStrippedSize {
	return &tg.PhotoStrippedSize{Type: "i", Bytes: []byte{1, 40, 30, 0xAB, 0xCD}}
}

func TestMedia_LinkPreview(t *testing.T) {
	t.Parallel()

	page := &tg.WebPage{URL: "https://x.io/a", DisplayURL: "x.io/a"}
	page.SetSiteName("X")
	page.SetTitle("A page")
	page.SetDescription("About it")
	page.SetPhoto(&tg.Photo{Sizes: []tg.PhotoSizeClass{stripped()}})

	got := media(&tg.MessageMediaWebPage{Webpage: page})
	if got == nil || got.Kind != domain.MediaLink || got.URL != "https://x.io/a" || got.SiteName != "X" ||
		got.Title != "A page" || got.Description != "About it" {
		t.Fatalf("media = %+v, want the link preview", got)
	}

	jpeg, err := base64.StdEncoding.DecodeString(got.Thumb)
	if err != nil || !bytes.HasPrefix(jpeg, []byte{0xFF, 0xD8}) {
		t.Errorf("thumb is not a JPEG: %v", err)
	}
}

// photo is a photo with a stripped preview and two sizes, the larger
// progressive. It returns a fresh value each time, for the same reason
// as stripped.
func photo() *tg.Photo {
	return &tg.Photo{ID: 7, AccessHash: 8, FileReference: []byte("ref"), Sizes: []tg.PhotoSizeClass{
		stripped(),
		&tg.PhotoSize{Type: "m", W: 320, H: 240},
		&tg.PhotoSizeProgressive{Type: "y", W: 1280, H: 960},
	}}
}

func TestMedia_Photo(t *testing.T) {
	t.Parallel()

	got := media(&tg.MessageMediaPhoto{Photo: photo()})
	if got == nil || got.Kind != domain.MediaPhoto || got.Width != 1280 || got.Height != 960 || got.Thumb == "" {
		t.Errorf("media = %+v, want the photo at its largest size with a preview", got)
	}
}

func TestMedia_VideoFileAndVoice(t *testing.T) {
	t.Parallel()

	video := &tg.Document{Size: 1000, Thumbs: []tg.PhotoSizeClass{stripped()}, Attributes: []tg.DocumentAttributeClass{
		&tg.DocumentAttributeVideo{Duration: 65.4, W: 1280, H: 720},
		&tg.DocumentAttributeFilename{FileName: "clip.mp4"},
	}}
	file := &tg.Document{Size: 2048, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "report.pdf"}}}
	voice := &tg.Document{Size: 300, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: true, Duration: 5}}}

	tests := map[string]struct {
		doc  *tg.Document
		want domain.Media
	}{
		"video": {video, domain.Media{Kind: domain.MediaVideo, Width: 1280, Height: 720, Duration: 65, FileName: "clip.mp4", Size: 1000}},
		"file":  {file, domain.Media{Kind: domain.MediaFile, FileName: "report.pdf", Size: 2048}},
		"voice": {voice, domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg", Size: 300, Duration: 5}},
	}

	for name, tt := range tests {
		got := media(&tg.MessageMediaDocument{Document: tt.doc})
		if got == nil {
			t.Errorf("%s: no media", name)
			continue
		}

		thumb := got.Thumb
		got.Thumb = ""
		if *got != tt.want {
			t.Errorf("%s: media = %+v, want %+v", name, *got, tt.want)
		}

		if (name == "video") != (thumb != "") {
			t.Errorf("%s: thumb %q", name, thumb)
		}
	}
}

func TestMedia_Sticker(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		doc          *tg.Document
		wantFileName string
	}{
		"a static WebP sticker is fetchable": {
			&tg.Document{
				Size: 500, MimeType: "image/webp", Thumbs: []tg.PhotoSizeClass{stripped()},
				Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "😀"}},
			},
			"sticker.webp",
		},
		"a Lottie animation is never fetched": {
			&tg.Document{
				Size: 500, MimeType: "application/x-tgsticker",
				Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "😀"}},
			},
			"",
		},
		"a WebM video sticker is never fetched": {
			&tg.Document{
				Size: 500, MimeType: "video/webm",
				Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "😀"}},
			},
			"",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := media(&tg.MessageMediaDocument{Document: tt.doc})
			if got == nil || got.Kind != domain.MediaSticker {
				t.Fatalf("media = %+v, want a sticker", got)
			}
			if got.FileName != tt.wantFileName {
				t.Errorf("FileName = %q, want %q", got.FileName, tt.wantFileName)
			}
			if got.Emoji != "😀" {
				t.Errorf("Emoji = %q, want the sticker's alt emoji", got.Emoji)
			}
		})
	}
}

func TestMessageText_LabelsDocumentsByWhatTheyAre(t *testing.T) {
	t.Parallel()

	tests := map[string]tg.DocumentAttributeClass{
		"[Video]":         &tg.DocumentAttributeVideo{},
		"[Voice message]": &tg.DocumentAttributeAudio{Voice: true},
		"[Sticker]":       &tg.DocumentAttributeSticker{},
		"[File]":          &tg.DocumentAttributeFilename{FileName: "a.pdf"},
	}

	for want, attr := range tests {
		doc := &tg.MessageMediaDocument{Document: &tg.Document{Attributes: []tg.DocumentAttributeClass{attr}}}
		if got := messageText(&tg.Message{Media: doc}); got != want {
			t.Errorf("messageText(%T) = %q, want %q", attr, got, want)
		}
	}
}

func TestMedia_NothingToShow(t *testing.T) {
	t.Parallel()

	for name, m := range map[string]tg.MessageMediaClass{
		"no media":                               nil,
		"a preview not ready":                    &tg.MessageMediaWebPage{Webpage: &tg.WebPagePending{}},
		"a preview with no title or description": &tg.MessageMediaWebPage{Webpage: &tg.WebPage{URL: "https://x.io"}},
		"a photo since deleted":                  &tg.MessageMediaPhoto{Photo: &tg.PhotoEmpty{}},
	} {
		if got := media(m); got != nil {
			t.Errorf("%s: media = %+v, want none", name, got)
		}
	}
}

func TestFetchMedia_DownloadsTheLargestPhotoWithAFreshReference(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		conv domain.Conversation
		get  bin.Encoder
	}{
		{"direct chat", chatWithNadia, &tg.MessagesGetMessagesRequest{}},
		{"channel", domain.Conversation{AccountID: "tg", RemoteID: "channel:5:3"}, &tg.ChannelsGetMessagesRequest{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFakeTelegram()
			f.reply(tt.get, &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaPhoto{Photo: photo()}}}})
			f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: []byte("jpeg")})

			path := filepath.Join(t.TempDir(), "m1.jpg")
			if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), tt.conv, "40", path); err != nil {
				t.Fatal(err)
			}

			if got, err := os.ReadFile(path); err != nil || string(got) != "jpeg" {
				t.Errorf("downloaded %q, %v", got, err)
			}

			var location *tg.InputPhotoFileLocation
			for _, r := range f.sent() {
				if get, ok := r.(*tg.UploadGetFileRequest); ok {
					location, _ = get.Location.(*tg.InputPhotoFileLocation)
				}
			}
			if location == nil || location.ID != 7 || location.ThumbSize != "y" || string(location.FileReference) != "ref" {
				t.Errorf("downloaded from %+v, want the largest size of photo 7", location)
			}
		})
	}
}

func TestFetchMedia_Fails(t *testing.T) {
	t.Parallel()

	noMedia := newFakeTelegram()
	noMedia.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}}}})
	gone := newFakeTelegram()
	gone.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{})

	path := filepath.Join(t.TempDir(), "x")
	for name, err := range map[string]error{
		"before signing in":          New(domain.Account{ID: "tg"}, "").FetchMedia(t.Context(), chatWithNadia, "40", path),
		"for a malformed id":         connectedTo(newFakeTelegram(), &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "x", path),
		"for a malformed peer":       connectedTo(newFakeTelegram(), &connectortest.Sink{}).FetchMedia(t.Context(), domain.Conversation{RemoteID: "bad"}, "40", path),
		"when Telegram refuses":      connectedTo(newFakeTelegram(), &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path),
		"for a message with nothing": connectedTo(noMedia, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path),
		"for a deleted message":      connectedTo(gone, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path),
	} {
		if err == nil {
			t.Errorf("FetchMedia %s succeeded, want an error", name)
		}
	}
}

func TestFileLocation_ForADocument(t *testing.T) {
	t.Parallel()

	got, err := fileLocation(&tg.MessageMediaDocument{Document: &tg.Document{ID: 9, AccessHash: 10, FileReference: []byte("ref")}})
	doc, ok := got.(*tg.InputDocumentFileLocation)
	if err != nil || !ok || doc.ID != 9 || doc.AccessHash != 10 || string(doc.FileReference) != "ref" {
		t.Errorf("fileLocation = %+v, %v; want document 9", got, err)
	}
}
