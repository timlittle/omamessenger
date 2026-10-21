package whatsapp

// FetchMedia is driven through a fake device and the in-memory media
// store connectedToWithMedia wires in, because the fake is the only way
// to see what Connector asked whatsmeow to download without reaching
// WhatsApp's servers.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/synctest"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestFetchMedia_DownloadsAndDecryptsAStoredReference covers the
// generic download-decrypt-write mechanics with a document reference:
// a kind that reaches the UI as a plain file, never a QML Image, so
// this test's own bytes stay an arbitrary placeholder rather than a
// real image. A photo, sticker or GIF's own route through imagesafe is
// covered separately below.
func TestFetchMedia_DownloadsAndDecryptsAStoredReference(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = []byte("decrypted bytes")
	ref := mediaRef{
		Kind: mediaKindDocument, DirectPath: "/v/doc", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), FileLength: 16, Mimetype: "application/pdf", FileName: "report.pdf",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "decrypted bytes" {
		t.Errorf("downloaded %q, %v, want the decrypted bytes written", got, err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, %v, want 0600", info, err)
	}

	if len(dev.downloadCalls) != 1 || !reflect.DeepEqual(dev.downloadCalls[0], ref) {
		t.Errorf("downloadCalls = %+v, want the stored reference", dev.downloadCalls)
	}
}

// TestFetchMedia_SanitizesADownloadedPhotoStickerAndGIF covers every
// kind this connector lets the UI load into a QML Image or
// AnimatedImage - a photo, a sticker and a document that turned out
// to be a real GIF (see isGIFImage) - proving each one routes through
// imagesafe rather than writing whatsmeow's downloaded bytes straight
// to disk.
func TestFetchMedia_SanitizesADownloadedPhotoStickerAndGIF(t *testing.T) {
	t.Parallel()

	tests := map[string]mediaRef{
		"photo":   {Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"), FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/jpeg"},
		"sticker": {Kind: mediaKindSticker, DirectPath: "/v/sticker", MediaKey: []byte("key"), FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/webp"},
		"gif as a document": {
			Kind: mediaKindDocument, DirectPath: "/v/gif", MediaKey: []byte("key"), FileSHA256: []byte{}, FileEncSHA256: []byte{},
			Mimetype: "image/gif", FileName: "nice.gif",
		},
	}

	for name, ref := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dev, _, c, media := connectedMediaFixture(t)
			dev.downloadData = tinyJPEG(t) // imagesafe detects the real format by magic bytes, not ref.Mimetype
			if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(t.TempDir(), "out")
			if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err != nil {
				t.Fatalf("FetchMedia() = %v, want a sanitized file written", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read sanitized file: %v", err)
			}
			if _, err := jpegDecodeConfig(got); err != nil {
				t.Errorf("sanitized file does not decode as an image: %v", err)
			}
		})
	}
}

// TestFetchMedia_RejectsAnUnsafeDownloadedPhoto covers a photo
// download that comes back as something other than a real image: the
// kind every QML Image component ever loads, so writing it to disk
// unchecked is exactly the class a marketplace review flagged this
// connector's sticker thumbnail for. No file must ever land on disk
// for it.
func TestFetchMedia_RejectsAnUnsafeDownloadedPhoto(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = []byte("not an image at all")
	ref := mediaRef{
		Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"),
		FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/jpeg",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err == nil {
		t.Fatal("FetchMedia() = nil, want an error for unsafe image bytes")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("unsafe image bytes were written to disk anyway")
	}
}

// TestFetchMedia_RefusesADeclaredSizeOverTheCap covers a photo
// reference whose own declared FileLength already exceeds the image
// profile's input cap: FetchMedia must refuse before ever asking
// whatsmeow to download it, not after buffering the whole file into
// memory first.
func TestFetchMedia_RefusesADeclaredSizeOverTheCap(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	ref := mediaRef{
		Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"),
		FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/jpeg",
		FileLength: uint64(imagesafe.FullImage.MaxInputLen) + 1,
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err == nil {
		t.Fatal("FetchMedia() with an oversized declared size succeeded, want it refused")
	}

	if len(dev.downloadCalls) != 0 {
		t.Errorf("downloadCalls = %+v, want nothing downloaded for an oversized declared size", dev.downloadCalls)
	}
}

// TestFetchMedia_RejectsADownloadLargerThanItsDeclaredSize covers a
// sender whose declared FileLength passes the cheap check above, but
// whose actual bytes do not: boundedFile's own streamed cap is the
// backstop that refuses it, well before imagesafe ever sees a byte
// (see fetch.go's streamImageOnce).
func TestFetchMedia_RejectsADownloadLargerThanItsDeclaredSize(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = bytes.Repeat([]byte{0x42}, int(imagesafe.Thumbnail.MaxInputLen)+1)
	ref := mediaRef{
		Kind: mediaKindSticker, DirectPath: "/v/sticker", MediaKey: []byte("key"),
		FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/webp", FileLength: 500,
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err == nil {
		t.Fatal("FetchMedia() with a download larger than its declared size succeeded, want it refused")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("FetchMedia() with an oversized download left a file behind")
	}
}

// TestFetchMedia_DownloadsAnAudioReference confirms a voice note or other
// audio message, saved with mediaKindAudio, downloads the same way a
// photo does: the fake device sees the exact reference saved, with no
// kind-based rejection anywhere between the store and the device call.
func TestFetchMedia_DownloadsAnAudioReference(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = []byte("decrypted voice note")
	ref := mediaRef{
		Kind: mediaKindAudio, DirectPath: "/v/voice", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), FileLength: 9, Mimetype: "audio/ogg; codecs=opus",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-voice", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.ogg")
	if err := c.FetchMedia(t.Context(), directChat, "msg-voice", path, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "decrypted voice note" {
		t.Errorf("downloaded %q, %v, want the decrypted bytes written", got, err)
	}

	if len(dev.downloadCalls) != 1 || !reflect.DeepEqual(dev.downloadCalls[0], ref) {
		t.Errorf("downloadCalls = %+v, want the stored audio reference", dev.downloadCalls)
	}
}

// TestFetchMedia_ReportsDecryptFailureForAWornOutReference confirms
// whatsmeow's own hash and HMAC errors, which mean a downloaded file no
// longer matches the key or hash the message carried, are reported as
// domain.ErrMediaDecryptFailed rather than left as a plain download
// failure, so the helper's log and the UI's tooltip can say which one it
// was.
func TestFetchMedia_ReportsDecryptFailureForAWornOutReference(t *testing.T) {
	t.Parallel()

	for _, want := range []error{
		whatsmeow.ErrInvalidMediaHMAC, whatsmeow.ErrInvalidMediaEncSHA256,
		whatsmeow.ErrInvalidMediaSHA256, whatsmeow.ErrInvalidUnencryptedMediaSHA256,
	} {
		dev, _, c, media := connectedMediaFixture(t)
		dev.downloadErr = want
		if err := media.put(t.Context(), directChat.RemoteID, "msg-1", savedRef()); err != nil {
			t.Fatal(err)
		}

		err := c.FetchMedia(t.Context(), directChat, "msg-1", filepath.Join(t.TempDir(), "x"), false)
		if !errors.Is(err, domain.ErrMediaDecryptFailed) {
			t.Errorf("FetchMedia with %v = %v, want it to also report domain.ErrMediaDecryptFailed", want, err)
		}
		if !errors.Is(err, want) {
			t.Errorf("FetchMedia with %v = %v, want whatsmeow's own error kept in the chain", want, err)
		}
	}
}

// TestFetchMedia_AcceptsAFileThatOnlyFailsThePlaintextDigest confirms a
// streamed, non-image download (savedRef is mediaKindDocument - see
// fetch_stream.go) that decrypted successfully, but no longer matches
// the plaintext hash its message declared, is still written out:
// whatsmeow already authenticated it with the media-key HMAC before
// running that specific check (see recoverableStaleDigest in
// fetch.go), and WhatsApp's own apps accept a stale digest the same
// way. TestFetchMedia_AcceptsAPhotoThatOnlyFailsThePlaintextDigest
// covers the same recovery for the streamed image path.
func TestFetchMedia_AcceptsAFileThatOnlyFailsThePlaintextDigest(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = []byte("decrypted anyway")
	dev.downloadErr = whatsmeow.ErrInvalidMediaSHA256
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", savedRef()); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "x")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err != nil {
		t.Fatalf("FetchMedia = %v, want a stale plaintext digest to be accepted", err)
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "decrypted anyway" {
		t.Errorf("downloaded %q, %v, want the decrypted bytes written despite the stale digest", got, err)
	}
}

// TestFetchMedia_AcceptsAPhotoThatOnlyFailsThePlaintextDigest is
// TestFetchMedia_AcceptsAFileThatOnlyFailsThePlaintextDigest's own
// streamed-image counterpart: a photo whose plaintext no longer
// matches its declared hash is still sanitized and written, through
// streamImageOrRetry's own recoverableStaleDigest check, with no
// retry needed at all.
func TestFetchMedia_AcceptsAPhotoThatOnlyFailsThePlaintextDigest(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = tinyJPEG(t)
	dev.downloadErr = whatsmeow.ErrInvalidMediaSHA256
	ref := mediaRef{Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"), FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/jpeg"}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err != nil {
		t.Fatalf("FetchMedia = %v, want a stale plaintext digest to be accepted", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded photo: %v", err)
	}
	if _, err := jpegDecodeConfig(got); err != nil {
		t.Errorf("downloaded photo does not decode: %v", err)
	}
}

func TestFetchMedia_ReportsNotFoundForAnUnsavedMessage(t *testing.T) {
	t.Parallel()

	dev, _, c, _ := connectedMediaFixture(t)

	err := c.FetchMedia(t.Context(), directChat, "missing", filepath.Join(t.TempDir(), "x"), false)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FetchMedia = %v, want domain.ErrNotFound", err)
	}
	if len(dev.downloadCalls) != 0 {
		t.Errorf("downloadCalls = %+v, want nothing downloaded for an unsaved reference", dev.downloadCalls)
	}
}

func TestFetchMedia_FailsWhenNotConnected(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa-1"}, t.TempDir())

	err := c.FetchMedia(t.Context(), directChat, "msg-1", filepath.Join(t.TempDir(), "x"), false)
	if !errors.Is(err, errNotConnected) {
		t.Errorf("FetchMedia = %v, want errNotConnected", err)
	}
}

// TestFetchMedia_RetriesAnExpiredLinkForAPhotoThroughThePhone covers
// retryStreamedImageDownload - a photo, sticker or real GIF file,
// streamed to a temporary file and sanitized through imagesafe once
// downloaded (see fetch.go's fetchImage) - the image-kind counterpart
// of retry_test.go's own
// TestFetchMedia_RetriesAnExpiredLinkThroughThePhone, which only
// exercises a document's own streamed retry (retryStreamedDownload).
func TestFetchMedia_RetriesAnExpiredLinkForAPhotoThroughThePhone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		key := testMediaKey(2)
		dev, sink, c, media := connectedMediaFixture(t)
		dev.downloadErrPaths = map[string]error{"/old": whatsmeow.ErrMediaDownloadFailedWith410}
		dev.downloadData = tinyJPEG(t)
		unregister := c.handleEvents(t.Context(), dev, media, sink)
		defer unregister()

		if err := media.putMessageKey(t.Context(), directChat.RemoteID, "msg-photo", messageKey{senderID: remoteID(directPeer)}); err != nil {
			t.Fatal(err)
		}
		ref := mediaRef{Kind: mediaKindImage, DirectPath: "/old", MediaKey: key, FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), Mimetype: "image/jpeg"}
		if err := media.put(t.Context(), directChat.RemoteID, "msg-photo", ref); err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(t.TempDir(), "out.jpg")
		done := make(chan error, 1)
		go func() { done <- c.FetchMedia(t.Context(), directChat, "msg-photo", path, false) }()
		synctest.Wait()

		if len(dev.mediaRetryCalls) != 1 {
			t.Fatalf("mediaRetryCalls = %d, want exactly one retry receipt sent", len(dev.mediaRetryCalls))
		}

		dev.fireEvent(mediaRetryEvent(t, key, "msg-photo", &waMmsRetry.MediaRetryNotification{
			Result: waMmsRetry.MediaRetryNotification_SUCCESS.Enum(), DirectPath: strPtr("/new"),
		}))
		synctest.Wait()

		if err := <-done; err != nil {
			t.Fatalf("FetchMedia = %v, want it to succeed once the phone answers", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read downloaded photo: %v", err)
		}
		if _, err := jpegDecodeConfig(got); err != nil {
			t.Errorf("downloaded photo does not decode: %v", err)
		}
	})
}

func TestFetchMedia_WrapsADownloadError(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadErr = errors.New("media server unavailable")
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", savedRef()); err != nil {
		t.Fatal(err)
	}

	err := c.FetchMedia(t.Context(), directChat, "msg-1", filepath.Join(t.TempDir(), "x"), false)
	if !errors.Is(err, dev.downloadErr) {
		t.Errorf("FetchMedia = %v, want it to wrap the device's error", err)
	}
}

// savedRef is a document reference complete enough to pass media's
// NOT NULL columns, for tests that only care that a reference exists
// to look up.
func savedRef() mediaRef {
	return mediaRef{Kind: mediaKindDocument, MediaKey: []byte("key"), FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc")}
}
