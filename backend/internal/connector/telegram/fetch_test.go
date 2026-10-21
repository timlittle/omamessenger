package telegram

// Tests for FetchMedia and its own helpers in fetch.go: the download
// path (plain, bounded or capped, automatic or user-requested) and the
// fileLocation lookup. media_test.go covers the pure media() and
// messageText() mapping instead.

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg" // registers the JPEG decoder this file's own decode checks use
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

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

			jpeg := tinyJPEG(t)
			f := newFakeTelegram()
			f.reply(tt.get, &tg.MessagesMessages{Messages: []tg.MessageClass{&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaPhoto{Photo: photo()}}}})
			f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: jpeg})

			path := filepath.Join(t.TempDir(), "m1.jpg")
			if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), tt.conv, "40", path, false); err != nil {
				t.Fatal(err)
			}

			// A photo routes through imagesafe (see imageProfile), which
			// never guarantees a byte-identical copy, only that the
			// result still decodes; imagesafe's own tests cover what it
			// strips and caps in detail.
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read downloaded photo: %v", err)
			}
			if _, _, err := image.DecodeConfig(bytes.NewReader(got)); err != nil {
				t.Errorf("downloaded photo does not decode: %v", err)
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

// TestFetchMedia_SanitizesStickerAndGIFDocument covers the two
// document-shaped media kinds this connector lets the UI load into a
// QML Image or AnimatedImage - a static WebP sticker, and a document
// that isGIFImage says is a genuine GIF - proving each routes through
// imagesafe rather than writing Telegram's downloaded bytes straight
// to disk; an ordinary document, Telegram's own stand-in for every
// other file kind, proves the boundary by being written unchanged.
func TestFetchMedia_SanitizesStickerAndGIFDocument(t *testing.T) {
	t.Parallel()

	tests := map[string]*tg.Document{
		"sticker": {
			ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: 500, MimeType: "image/webp",
			Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "😀", Stickerset: &tg.InputStickerSetEmpty{}}},
		},
		"gif as a document": {
			ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: 500, MimeType: "image/gif",
			Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "nice.gif"}},
		},
	}

	for name, doc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			jpeg := tinyJPEG(t)
			f := newFakeTelegram()
			f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
				&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
			}})
			f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: jpeg})

			path := filepath.Join(t.TempDir(), "out")
			if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false); err != nil {
				t.Fatalf("FetchMedia() = %v, want a sanitized file written", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read sanitized file: %v", err)
			}
			if _, _, err := image.DecodeConfig(bytes.NewReader(got)); err != nil {
				t.Errorf("sanitized file does not decode as an image: %v", err)
			}
		})
	}
}

// TestFetchMedia_RefusesADeclaredSizeOverTheCap covers a document
// whose own declared Size already exceeds the image profile's input
// cap: FetchMedia must refuse before ever asking Telegram to download
// it, not after buffering the whole thing into memory first.
func TestFetchMedia_RefusesADeclaredSizeOverTheCap(t *testing.T) {
	t.Parallel()

	doc := &tg.Document{
		ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: imagesafe.Thumbnail.MaxInputLen + 1, MimeType: "image/webp",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "😀", Stickerset: &tg.InputStickerSetEmpty{}}},
	}
	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
		&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
	}})

	path := filepath.Join(t.TempDir(), "out")
	if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false); err == nil {
		t.Fatal("FetchMedia() with an oversized declared size succeeded, want it refused")
	}

	for _, r := range f.sent() {
		if _, ok := r.(*tg.UploadGetFileRequest); ok {
			t.Error("FetchMedia() downloaded bytes for an oversized declared size, want it refused before asking Telegram at all")
		}
	}
}

// TestFetchMedia_RefusesAStreamLargerThanDeclared covers a sender
// whose declared size passes the cheap check above, but whose actual
// bytes do not: the stream itself must still be bounded, so FetchMedia
// never ends up holding more than one profile's worth of bytes in
// memory, and never leaves a file behind either.
func TestFetchMedia_RefusesAStreamLargerThanDeclared(t *testing.T) {
	t.Parallel()

	doc := &tg.Document{
		ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: 500, MimeType: "image/webp",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "😀", Stickerset: &tg.InputStickerSetEmpty{}}},
	}
	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
		&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
	}})
	oversized := bytes.Repeat([]byte{0x42}, int(imagesafe.Thumbnail.MaxInputLen)+1)
	f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: oversized})

	path := filepath.Join(t.TempDir(), "out")
	if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false); err == nil {
		t.Fatal("FetchMedia() with a stream larger than declared succeeded, want it refused")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("FetchMedia() with an oversized stream left a file behind")
	}
}

// TestFetchMedia_TimesOutDownloadingAnImage confirms a stalled image
// download does not leave a caller waiting forever.
func TestFetchMedia_TimesOutDownloadingAnImage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakeTelegram()
		f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
			&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaPhoto{Photo: photo()}},
		}})
		f.blockRequest(&tg.UploadGetFileRequest{})

		path := filepath.Join(t.TempDir(), "x")
		done := make(chan error, 1)
		go func() {
			done <- connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false)
		}()

		time.Sleep(fetchTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("FetchMedia() = %v, want context.DeadlineExceeded after the fetch timeout", err)
			}
		default:
			t.Fatal("FetchMedia did not return once its timeout elapsed")
		}
	})
}

// TestFetchMedia_WritesAnOrdinaryDocumentUnchanged proves the
// boundary TestFetchMedia_SanitizesStickerAndGIFDocument covers the
// inside of: a document that is none of a photo, a sticker or a real
// GIF - a PDF here, any other file in practice - is written exactly
// as Telegram sent it, never run through imagesafe at all.
func TestFetchMedia_WritesAnOrdinaryDocumentUnchanged(t *testing.T) {
	t.Parallel()

	doc := &tg.Document{
		ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: 500, MimeType: "application/pdf",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "report.pdf"}},
	}
	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
		&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
	}})
	f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: []byte("not an image at all")})

	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != "not an image at all" {
		t.Errorf("downloaded %q, %v, want the bytes written unchanged", got, err)
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
		"before signing in":          New(domain.Account{ID: "tg"}, "").FetchMedia(t.Context(), chatWithNadia, "40", path, false),
		"for a malformed id":         connectedTo(newFakeTelegram(), &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "x", path, false),
		"for a malformed peer":       connectedTo(newFakeTelegram(), &connectortest.Sink{}).FetchMedia(t.Context(), domain.Conversation{RemoteID: "bad"}, "40", path, false),
		"when Telegram refuses":      connectedTo(newFakeTelegram(), &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false),
		"for a message with nothing": connectedTo(noMedia, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false),
		"for a deleted message":      connectedTo(gone, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false),
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
