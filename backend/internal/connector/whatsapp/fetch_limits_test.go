package whatsapp

// Tests for FetchMedia's auto and size-limit behaviour: the
// automatic-fetch cap an animated video's own autoplay uses, the
// larger user-requested cap a deliberate download of a video, file or
// voice note gets instead, and the idle timeout that bounds a stalled
// streamed download (see fetch.go, fetch_stream.go and
// download_file.go).

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// TestFetchMedia_DownloadsAnAutomaticFetchUnderTheCap confirms the
// animated-video autoplay path succeeds normally once the declared
// size fits within connector.AutoFetchMaxBytes, exercising the same
// flat fetchTimeout deadline an automatic fetch used before streaming
// existed (see downloadDeadline in fetch_stream.go).
func TestFetchMedia_DownloadsAnAutomaticFetchUnderTheCap(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = []byte("a short silent clip")
	ref := mediaRef{
		Kind: mediaKindVideo, DirectPath: "/v/gif", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), FileLength: 20, Mimetype: "video/mp4",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.mp4")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, true); err != nil {
		t.Fatalf("FetchMedia(auto) = %v, want a declared size under the cap allowed", err)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != "a short silent clip" {
		t.Errorf("downloaded %q, %v, want the bytes written unchanged", got, err)
	}
}

// TestFetchMedia_RefusesAnAutomaticFetchOverTheAutoCap covers the
// animated-video autoplay path: a video whose declared size already
// exceeds connector.AutoFetchMaxBytes is refused before whatsmeow is
// ever asked to download it, when auto is true, even though the same
// file is allowed for a deliberate click (see
// TestFetchMedia_AllowsAUserRequestedFetchOverTheAutoCap).
func TestFetchMedia_RefusesAnAutomaticFetchOverTheAutoCap(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	ref := mediaRef{
		Kind: mediaKindVideo, DirectPath: "/v/gif", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"),
		FileLength: uint64(connector.AutoFetchMaxBytes) + 1, Mimetype: "video/mp4",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.mp4")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, true); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("FetchMedia(auto) = %v, want connector.ErrMediaTooLarge", err)
	}
	if len(dev.downloadCalls) != 0 {
		t.Errorf("downloadCalls = %+v, want nothing downloaded over the auto cap", dev.downloadCalls)
	}
}

// TestFetchMedia_AllowsAUserRequestedFetchOverTheAutoCap confirms the
// same declared size TestFetchMedia_RefusesAnAutomaticFetchOverTheAutoCap
// refuses for an automatic fetch is still allowed once the user clicks:
// auto false applies connector.MaxUserFetchBytes instead.
func TestFetchMedia_AllowsAUserRequestedFetchOverTheAutoCap(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = []byte("a whole video, trust me")
	ref := mediaRef{
		Kind: mediaKindVideo, DirectPath: "/v/gif", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"),
		FileLength: uint64(connector.AutoFetchMaxBytes) + 1, Mimetype: "video/mp4",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.mp4")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err != nil {
		t.Fatalf("FetchMedia(user) = %v, want the declared size over the auto cap still allowed", err)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != "a whole video, trust me" {
		t.Errorf("downloaded %q, %v, want the bytes written unchanged", got, err)
	}
}

// TestFetchMedia_CapsAFileStreamLargerThanDeclared covers a video,
// file or voice note whose declared size passes the cheap check, but
// whose actual bytes do not: boundedFile must still refuse it, and
// never leave a file behind.
func TestFetchMedia_CapsAFileStreamLargerThanDeclared(t *testing.T) {
	t.Parallel()

	const declared = 500
	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = bytes.Repeat([]byte{0x42}, int(connector.ByteCap(declared, connector.MaxUserFetchBytes))+1)
	ref := mediaRef{
		Kind: mediaKindDocument, DirectPath: "/v/doc", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), FileLength: declared, Mimetype: "application/pdf", FileName: "report.pdf",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("FetchMedia() = %v, want connector.ErrMediaTooLarge for a stream larger than declared", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("FetchMedia() with an oversized stream left a file behind")
	}
}

// TestFetchMedia_RefusesADeclaredFileSizeOverTheUserCap covers a video,
// file or voice note whose own declared FileLength already exceeds
// connector.MaxUserFetchBytes: refused before whatsmeow is asked to
// download anything, the streamed-path counterpart of
// TestFetchMedia_RefusesADeclaredSizeOverTheCap for images.
func TestFetchMedia_RefusesADeclaredFileSizeOverTheUserCap(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	ref := mediaRef{
		Kind: mediaKindVideo, DirectPath: "/v/huge", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"),
		FileLength: uint64(connector.MaxUserFetchBytes) + 1, Mimetype: "video/mp4",
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.mp4")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("FetchMedia() = %v, want connector.ErrMediaTooLarge", err)
	}
	if len(dev.downloadCalls) != 0 {
		t.Errorf("downloadCalls = %+v, want nothing downloaded for a declared size over the user cap", dev.downloadCalls)
	}
}

// TestFetchMedia_TimesOutWhenWhatsAppNeverAnswers confirms a stalled,
// user-requested document download (savedRef is mediaKindDocument,
// which streams to disk rather than buffering in memory - see
// fetch_stream.go) does not leave a caller waiting forever: it is cut
// off by the idle timeout, well before the far longer absolute
// ceiling. A manual idle cancel reports context.Canceled, not
// context.DeadlineExceeded (see connector.IdleContext); app's own
// failure classification already treats both the same way, as a
// timeout.
func TestFetchMedia_TimesOutWhenWhatsAppNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, _, c, media := connectedMediaFixture(t)
		dev.downloadBlocks = true
		defer func() { _ = media.close() }() // stop its connection-opener goroutine before the bubble ends
		if err := media.put(t.Context(), directChat.RemoteID, "msg-1", savedRef()); err != nil {
			t.Fatal(err)
		}

		done := make(chan error, 1)
		path := filepath.Join(t.TempDir(), "x")
		go func() { done <- c.FetchMedia(t.Context(), directChat, "msg-1", path, false) }()

		time.Sleep(connector.IdleTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("FetchMedia = %v, want context.Canceled after the idle timeout", err)
			}
		default:
			t.Fatal("FetchMedia did not return once its idle timeout elapsed")
		}
	})
}

// TestFetchMedia_CapsAnImageStreamWithNoDeclaredSize covers the
// vulnerability a security review found: a sender who declares
// FileLength 0 must not be able to force a photo, sticker or GIF
// document's full bytes into memory unbounded. fetchImage now streams
// to a temporary file capped at the image profile's own MaxInputLen,
// so an undeclared size is caught the same way an undersold one
// already is, never read into memory in full first.
func TestFetchMedia_CapsAnImageStreamWithNoDeclaredSize(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = bytes.Repeat([]byte{0x42}, int(imagesafe.Thumbnail.MaxInputLen)+1)
	ref := mediaRef{
		Kind: mediaKindSticker, DirectPath: "/v/sticker", MediaKey: []byte("key"),
		FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/webp", FileLength: 0,
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("FetchMedia() = %v, want connector.ErrMediaTooLarge for an undeclared, oversized sticker", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("FetchMedia() with an undeclared, oversized sticker left a file behind")
	}
}

// TestFetchMedia_NarrowsAnAutomaticImageFetchToTheAutoCap confirms a
// photo shown automatically (its own thumbnail-to-full-image fetch,
// never a click) is bounded by connector.AutoFetchMaxBytes, the same
// as any other automatic fetch, even though imagesafe.FullImage's own
// MaxInputLen alone would allow more.
func TestFetchMedia_NarrowsAnAutomaticImageFetchToTheAutoCap(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	ref := mediaRef{
		Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"),
		FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/jpeg",
		FileLength: uint64(connector.AutoFetchMaxBytes) + 1,
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, true); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("FetchMedia(auto) = %v, want connector.ErrMediaTooLarge over the auto cap", err)
	}
	if len(dev.downloadCalls) != 0 {
		t.Errorf("downloadCalls = %+v, want nothing downloaded over the auto cap", dev.downloadCalls)
	}
}

// TestFetchMedia_AllowsADeliberateImageFetchOverTheAutoCap confirms the
// same declared size TestFetchMedia_NarrowsAnAutomaticImageFetchToTheAutoCap
// refuses for an automatic fetch is still allowed once the user clicks:
// auto false applies imagesafe.FullImage's own, larger MaxInputLen
// instead of connector.AutoFetchMaxBytes.
func TestFetchMedia_AllowsADeliberateImageFetchOverTheAutoCap(t *testing.T) {
	t.Parallel()

	dev, _, c, media := connectedMediaFixture(t)
	dev.downloadData = tinyJPEG(t)
	ref := mediaRef{
		Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"),
		FileSHA256: []byte{}, FileEncSHA256: []byte{}, Mimetype: "image/jpeg",
		FileLength: uint64(connector.AutoFetchMaxBytes) + 1,
	}
	if err := media.put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path, false); err != nil {
		t.Fatalf("FetchMedia(user) = %v, want a declared size over the auto cap still allowed", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded photo: %v", err)
	}
	if _, err := jpegDecodeConfig(got); err != nil {
		t.Errorf("downloaded photo does not decode: %v", err)
	}
}
