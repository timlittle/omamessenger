package whatsapp

// FetchMedia is driven through a fake device and the in-memory media
// store connectedToWithMedia wires in, because the fake is the only way
// to see what Connector asked whatsmeow to download without reaching
// WhatsApp's servers.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestFetchMedia_DownloadsAndDecryptsAStoredReference(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.downloadData = []byte("decrypted bytes")
	c := connectedToWithMedia(t, dev, &connectortest.Sink{})
	ref := mediaRef{
		Kind: mediaKindImage, DirectPath: "/v/photo", MediaKey: []byte("key"),
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"), FileLength: 16, Mimetype: "image/jpeg",
	}
	if err := c.mediaFor().put(t.Context(), directChat.RemoteID, "msg-1", ref); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.jpg")
	if err := c.FetchMedia(t.Context(), directChat, "msg-1", path); err != nil {
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

func TestFetchMedia_ReportsNotFoundForAnUnsavedMessage(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	c := connectedToWithMedia(t, dev, &connectortest.Sink{})

	err := c.FetchMedia(t.Context(), directChat, "missing", filepath.Join(t.TempDir(), "x"))
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

	err := c.FetchMedia(t.Context(), directChat, "msg-1", filepath.Join(t.TempDir(), "x"))
	if !errors.Is(err, errNotConnected) {
		t.Errorf("FetchMedia = %v, want errNotConnected", err)
	}
}

func TestFetchMedia_WrapsADownloadError(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.downloadErr = errors.New("media server unavailable")
	c := connectedToWithMedia(t, dev, &connectortest.Sink{})
	if err := c.mediaFor().put(t.Context(), directChat.RemoteID, "msg-1", savedRef()); err != nil {
		t.Fatal(err)
	}

	err := c.FetchMedia(t.Context(), directChat, "msg-1", filepath.Join(t.TempDir(), "x"))
	if !errors.Is(err, dev.downloadErr) {
		t.Errorf("FetchMedia = %v, want it to wrap the device's error", err)
	}
}

func TestFetchMedia_TimesOutWhenWhatsAppNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.downloadBlocks = true
		c := connectedToWithMedia(t, dev, &connectortest.Sink{})
		defer func() { _ = c.mediaFor().close() }() // stop its connection-opener goroutine before the bubble ends
		if err := c.mediaFor().put(t.Context(), directChat.RemoteID, "msg-1", savedRef()); err != nil {
			t.Fatal(err)
		}

		done := make(chan error, 1)
		path := filepath.Join(t.TempDir(), "x")
		go func() { done <- c.FetchMedia(t.Context(), directChat, "msg-1", path) }()

		time.Sleep(fetchTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("FetchMedia = %v, want context.DeadlineExceeded after the fetch timeout", err)
			}
		default:
			t.Fatal("FetchMedia did not return once its timeout elapsed")
		}
	})
}

// savedRef is a document reference complete enough to pass media's
// NOT NULL columns, for tests that only care that a reference exists
// to look up.
func savedRef() mediaRef {
	return mediaRef{Kind: mediaKindDocument, MediaKey: []byte("key"), FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc")}
}
