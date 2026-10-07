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
var stripped = &tg.PhotoStrippedSize{Type: "i", Bytes: []byte{1, 40, 30, 0xAB, 0xCD}}

func TestMedia_LinkPreview(t *testing.T) {
	t.Parallel()

	page := &tg.WebPage{URL: "https://x.io/a", DisplayURL: "x.io/a"}
	page.SetSiteName("X")
	page.SetTitle("A page")
	page.SetDescription("About it")
	page.SetPhoto(&tg.Photo{Sizes: []tg.PhotoSizeClass{stripped}})

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
// progressive.
var photo = &tg.Photo{ID: 7, AccessHash: 8, FileReference: []byte("ref"), Sizes: []tg.PhotoSizeClass{
	stripped,
	&tg.PhotoSize{Type: "m", W: 320, H: 240},
	&tg.PhotoSizeProgressive{Type: "y", W: 1280, H: 960},
}}

func TestMedia_Photo(t *testing.T) {
	t.Parallel()

	got := media(&tg.MessageMediaPhoto{Photo: photo})
	if got == nil || got.Kind != domain.MediaPhoto || got.Width != 1280 || got.Height != 960 || got.Thumb == "" {
		t.Errorf("media = %+v, want the photo at its largest size with a preview", got)
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
			f.reply(tt.get, &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaPhoto{Photo: photo}}}})
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
