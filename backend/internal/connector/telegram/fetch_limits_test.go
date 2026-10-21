package telegram

// Tests for FetchMedia's auto and size-limit behaviour: the
// automatic-fetch cap an animated video's own autoplay uses, and the
// larger user-requested cap and idle timeout a deliberate download of
// a video or other non-image document gets instead (see fetch.go's
// download and downloadDeadline).

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
)

// TestFetchMedia_DownloadsAnAutomaticFetchUnderTheCap confirms the
// animated-video autoplay path succeeds normally once the declared
// size fits within connector.AutoFetchMaxBytes, exercising the same
// flat fetchTimeout deadline an automatic fetch used before download's
// own idle-timeout path existed (see downloadDeadline).
func TestFetchMedia_DownloadsAnAutomaticFetchUnderTheCap(t *testing.T) {
	t.Parallel()

	content := []byte("a short silent clip")
	doc := &tg.Document{
		ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: int64(len(content)), MimeType: "video/mp4",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAnimated{}},
	}
	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
		&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
	}})
	f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: content})

	path := filepath.Join(t.TempDir(), "out.mp4")
	if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, true); err != nil {
		t.Fatalf("FetchMedia(auto) = %v, want a declared size under the cap allowed", err)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != string(content) {
		t.Errorf("downloaded %q, %v, want the bytes written unchanged", got, err)
	}
}

// TestFetchMedia_RefusesAnAutomaticFetchOverTheAutoCap covers the
// animated-video autoplay path: a document whose declared size already
// exceeds connector.AutoFetchMaxBytes is refused before Telegram is
// ever asked to download it, when auto is true, even though the same
// file would be allowed for a deliberate click (see
// TestFetchMedia_AllowsAUserRequestedFetchOverTheAutoCap).
func TestFetchMedia_RefusesAnAutomaticFetchOverTheAutoCap(t *testing.T) {
	t.Parallel()

	doc := &tg.Document{
		ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: connector.AutoFetchMaxBytes + 1, MimeType: "video/mp4",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAnimated{}},
	}
	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
		&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
	}})

	path := filepath.Join(t.TempDir(), "out.mp4")
	c := connectedTo(f, &connectortest.Sink{})
	if err := c.FetchMedia(t.Context(), chatWithNadia, "40", path, true); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("FetchMedia(auto) = %v, want connector.ErrMediaTooLarge", err)
	}

	for _, r := range f.sent() {
		if _, ok := r.(*tg.UploadGetFileRequest); ok {
			t.Error("FetchMedia(auto) downloaded bytes over the auto cap, want it refused before asking Telegram at all")
		}
	}
}

// TestFetchMedia_AllowsAUserRequestedFetchOverTheAutoCap confirms the
// same declared size TestFetchMedia_RefusesAnAutomaticFetchOverTheAutoCap
// refuses for an automatic fetch is still allowed once the user clicks:
// auto false applies connector.MaxUserFetchBytes instead.
func TestFetchMedia_AllowsAUserRequestedFetchOverTheAutoCap(t *testing.T) {
	t.Parallel()

	content := bytes.Repeat([]byte{0x42}, 1024)
	doc := &tg.Document{
		ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: connector.AutoFetchMaxBytes + 1, MimeType: "video/mp4",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeAnimated{}},
	}
	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
		&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
	}})
	f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: content})

	path := filepath.Join(t.TempDir(), "out.mp4")
	if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false); err != nil {
		t.Fatalf("FetchMedia(user) = %v, want the declared size over the auto cap still allowed", err)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != string(content) {
		t.Errorf("downloaded %q, %v, want the bytes written unchanged", got, err)
	}
}

// TestFetchMedia_CapsADocumentStreamLargerThanDeclared covers a
// non-image document - a video or a plain file, never run through
// imagesafe - whose declared size passes the cheap check, but whose
// actual bytes do not: download's own connector.BoundedWriter must
// still refuse it, and never leave a file behind.
func TestFetchMedia_CapsADocumentStreamLargerThanDeclared(t *testing.T) {
	t.Parallel()

	const declared = 500
	doc := &tg.Document{
		ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: declared, MimeType: "application/pdf",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "report.pdf"}},
	}
	f := newFakeTelegram()
	f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
		&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
	}})
	oversized := bytes.Repeat([]byte{0x42}, int(connector.ByteCap(declared, connector.MaxUserFetchBytes))+1)
	f.reply(&tg.UploadGetFileRequest{}, &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: oversized})

	path := filepath.Join(t.TempDir(), "report.pdf")
	if err := connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false); !errors.Is(err, connector.ErrMediaTooLarge) {
		t.Fatalf("FetchMedia() = %v, want connector.ErrMediaTooLarge for a stream larger than declared", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("FetchMedia() with an oversized stream left a file behind")
	}
}

// TestFetchMedia_TimesOutAUserRequestedFileOnIdle confirms a stalled,
// user-requested document download is cut off by the idle timeout, not
// made to wait for the far longer absolute ceiling.
func TestFetchMedia_TimesOutAUserRequestedFileOnIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		doc := &tg.Document{
			ID: 7, AccessHash: 8, FileReference: []byte("ref"), Size: 500, MimeType: "application/pdf",
			Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "report.pdf"}},
		}
		f := newFakeTelegram()
		f.reply(&tg.MessagesGetMessagesRequest{}, &tg.MessagesMessages{Messages: []tg.MessageClass{
			&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 42}, Media: &tg.MessageMediaDocument{Document: doc}},
		}})
		f.blockRequest(&tg.UploadGetFileRequest{})

		path := filepath.Join(t.TempDir(), "report.pdf")
		done := make(chan error, 1)
		go func() {
			done <- connectedTo(f, &connectortest.Sink{}).FetchMedia(t.Context(), chatWithNadia, "40", path, false)
		}()

		time.Sleep(connector.IdleTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			// A manual cancel (see connector.IdleContext) always reports
			// context.Canceled, never context.DeadlineExceeded, which is
			// reserved for the context's own deadline actually elapsing
			// (the absolute ceiling, not idle); app.fetchFailureReason
			// already treats both the same way, as a timeout.
			if !errors.Is(err, context.Canceled) {
				t.Errorf("FetchMedia() = %v, want context.Canceled after the idle timeout", err)
			}
		default:
			t.Fatal("FetchMedia did not return once its idle timeout elapsed, want it not to wait for the absolute ceiling")
		}
	})
}
