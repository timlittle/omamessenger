package whatsapp

// retry.go's media retry flow is driven through FetchMedia itself,
// exactly as fetch_test.go drives a plain download, because that is
// what actually proves fetch.go, retry.go and events.go's routing
// agree on a message id: dev.fireEvent delivers an events.MediaRetry
// the same way whatsmeow's own dispatcher would, through handleEvents
// registered the way Run registers it.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/util/gcmutil"
	"go.mau.fi/whatsmeow/util/hkdfutil"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// testMediaKey is a fixed-size AES key for the tests in this file, byte
// seed alone deciding its content so two messages can be given two
// different keys.
func testMediaKey(seed byte) []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = seed
	}

	return key
}

// mediaRetryEvent builds the events.MediaRetry a real WhatsApp phone
// would send in answer to a retry request for messageRemoteID,
// encrypted the same way whatsmeow derives and uses the key
// (SendMediaRetryReceipt's own doc comment names the derivation), so
// decryptRetryNotification in retry.go exercises its real decrypt path
// rather than a stub.
func mediaRetryEvent(t *testing.T, mediaKey []byte, messageRemoteID string, notif *waMmsRetry.MediaRetryNotification) *events.MediaRetry {
	t.Helper()

	plaintext, err := proto.Marshal(notif)
	if err != nil {
		t.Fatal(err)
	}

	key := hkdfutil.SHA256(mediaKey, nil, []byte("WhatsApp Media Retry Notification"), 32)
	iv := make([]byte, 12)
	ciphertext, err := gcmutil.Encrypt(key, iv, plaintext, []byte(messageRemoteID))
	if err != nil {
		t.Fatal(err)
	}

	return &events.MediaRetry{Ciphertext: ciphertext, IV: iv, MessageID: types.MessageID(messageRemoteID)}
}

// seedRetryMessage records messageRemoteID's sender and media
// reference, the way a live message's arrival would have, for a test
// that then drives FetchMedia and a retry for it.
func seedRetryMessage(t *testing.T, media *mediaStore, messageRemoteID string, key []byte, path string) {
	t.Helper()

	if err := media.putMessageKey(t.Context(), directChat.RemoteID, messageRemoteID, messageKey{senderID: remoteID(directPeer)}); err != nil {
		t.Fatal(err)
	}

	ref := mediaRef{
		Kind: mediaKindDocument, DirectPath: path, MediaKey: key,
		FileSHA256: []byte("sha"), FileEncSHA256: []byte("enc"),
	}
	if err := media.put(t.Context(), directChat.RemoteID, messageRemoteID, ref); err != nil {
		t.Fatal(err)
	}
}

// TestFetchMedia_RetriesAnExpiredLinkThroughThePhone confirms a 410
// triggers exactly one retry receipt, and that a matching success
// answer with a fresh path leads to a second download succeeding and
// the stored reference being updated to that path.
func TestFetchMedia_RetriesAnExpiredLinkThroughThePhone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		key := testMediaKey(1)
		dev.downloadErrPaths = map[string]error{"/old": whatsmeow.ErrMediaDownloadFailedWith410}
		dev.downloadData = []byte("fresh bytes")

		var sink connectortest.Sink
		c := connectedToWithMedia(t, dev, &sink)
		media := c.mediaFor()
		unregister := c.handleEvents(t.Context(), dev, media, &sink)
		defer unregister()

		seedRetryMessage(t, media, "msg-1", key, "/old")

		path := filepath.Join(t.TempDir(), "out")
		done := make(chan error, 1)
		go func() { done <- c.FetchMedia(t.Context(), directChat, "msg-1", path) }()
		synctest.Wait()

		if len(dev.mediaRetryCalls) != 1 {
			t.Fatalf("mediaRetryCalls = %d, want exactly one retry receipt sent", len(dev.mediaRetryCalls))
		}

		dev.fireEvent(mediaRetryEvent(t, key, "msg-1", &waMmsRetry.MediaRetryNotification{
			Result: waMmsRetry.MediaRetryNotification_SUCCESS.Enum(), DirectPath: strPtr("/new"),
		}))
		synctest.Wait()

		if err := <-done; err != nil {
			t.Fatalf("FetchMedia = %v, want it to succeed once the phone answers", err)
		}

		got, err := os.ReadFile(path)
		if err != nil || string(got) != "fresh bytes" {
			t.Errorf("downloaded %q, %v, want the bytes fetched with the new path", got, err)
		}

		updated, ok, err := media.get(t.Context(), directChat.RemoteID, "msg-1")
		if err != nil || !ok || updated.DirectPath != "/new" {
			t.Errorf("stored ref = %+v, %v, %v, want its direct path updated to /new", updated, ok, err)
		}
	})
}

// TestFetchMedia_ReportsExpiredWhenThePhoneSaysTheMediaIsGone confirms
// a NOT_FOUND answer is reported as domain.ErrMediaExpired, the
// category the UI shows as "no longer on the phone".
func TestFetchMedia_ReportsExpiredWhenThePhoneSaysTheMediaIsGone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		key := testMediaKey(2)
		dev.downloadErr = whatsmeow.ErrMediaDownloadFailedWith404

		var sink connectortest.Sink
		c := connectedToWithMedia(t, dev, &sink)
		media := c.mediaFor()
		unregister := c.handleEvents(t.Context(), dev, media, &sink)
		defer unregister()

		seedRetryMessage(t, media, "msg-1", key, "/old")

		path := filepath.Join(t.TempDir(), "x")
		done := make(chan error, 1)
		go func() { done <- c.FetchMedia(t.Context(), directChat, "msg-1", path) }()
		synctest.Wait()

		dev.fireEvent(mediaRetryEvent(t, key, "msg-1", &waMmsRetry.MediaRetryNotification{
			Result: waMmsRetry.MediaRetryNotification_NOT_FOUND.Enum(),
		}))
		synctest.Wait()

		if err := <-done; !errors.Is(err, domain.ErrMediaExpired) {
			t.Errorf("FetchMedia = %v, want domain.ErrMediaExpired once the phone reports the media gone", err)
		}
	})
}

// TestFetchMedia_ReportsExpiredWhenThePhoneNeverAnswers confirms a
// retry nobody ever answers still returns, as domain.ErrMediaExpired,
// once mediaRetryTimeout elapses, rather than waiting forever.
func TestFetchMedia_ReportsExpiredWhenThePhoneNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.downloadErr = whatsmeow.ErrMediaDownloadFailedWith410

		var sink connectortest.Sink
		c := connectedToWithMedia(t, dev, &sink)
		media := c.mediaFor()
		seedRetryMessage(t, media, "msg-1", testMediaKey(3), "/old")

		path := filepath.Join(t.TempDir(), "x")
		done := make(chan error, 1)
		go func() { done <- c.FetchMedia(t.Context(), directChat, "msg-1", path) }()

		time.Sleep(mediaRetryTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, domain.ErrMediaExpired) {
				t.Errorf("FetchMedia = %v, want domain.ErrMediaExpired once the retry timeout elapses", err)
			}
		default:
			t.Fatal("FetchMedia did not return once the retry timeout elapsed")
		}
	})
}

// TestFetchMedia_ConcurrentRetriesEachResolveTheirOwnMessage confirms
// two FetchMedia calls waiting on their own retry at the same time are
// never confused: an answer for one message never resolves the other's
// wait, and each stored reference ends up with its own new path. This
// one test uses a real file-backed media store, not the in-memory one
// connectedToWithMedia wires in elsewhere: two goroutines against a
// single ":memory:" connection would each silently open their own
// separate, empty in-memory database, which a real per-account file
// never does.
func TestFetchMedia_ConcurrentRetriesEachResolveTheirOwnMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		key1, key2 := testMediaKey(4), testMediaKey(5)
		dev.downloadErrPaths = map[string]error{
			"/old-1": whatsmeow.ErrMediaDownloadFailedWith410,
			"/old-2": whatsmeow.ErrMediaDownloadFailedWith410,
		}
		dev.downloadData = []byte("ok")

		var sink connectortest.Sink
		media := newTestMediaStore(t)
		c := &Connector{account: domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp}, answers: make(chan answer, 1)}
		c.connected(dev, &sink, media)
		unregister := c.handleEvents(t.Context(), dev, media, &sink)
		defer unregister()

		seedRetryMessage(t, media, "msg-1", key1, "/old-1")
		seedRetryMessage(t, media, "msg-2", key2, "/old-2")

		done1, done2 := make(chan error, 1), make(chan error, 1)
		path1, path2 := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
		go func() { done1 <- c.FetchMedia(t.Context(), directChat, "msg-1", path1) }()
		go func() { done2 <- c.FetchMedia(t.Context(), directChat, "msg-2", path2) }()
		synctest.Wait()

		if len(dev.mediaRetryCalls) != 2 {
			t.Fatalf("mediaRetryCalls = %d, want two concurrent retry requests", len(dev.mediaRetryCalls))
		}

		// msg-2's answer arrives first; it must not resolve msg-1's wait.
		dev.fireEvent(mediaRetryEvent(t, key2, "msg-2", &waMmsRetry.MediaRetryNotification{
			Result: waMmsRetry.MediaRetryNotification_SUCCESS.Enum(), DirectPath: strPtr("/new-2"),
		}))
		synctest.Wait()

		select {
		case err := <-done1:
			t.Fatalf("FetchMedia(msg-1) resolved = %v, want it still waiting for its own answer", err)
		default:
		}

		dev.fireEvent(mediaRetryEvent(t, key1, "msg-1", &waMmsRetry.MediaRetryNotification{
			Result: waMmsRetry.MediaRetryNotification_SUCCESS.Enum(), DirectPath: strPtr("/new-1"),
		}))
		synctest.Wait()

		if err := <-done1; err != nil {
			t.Fatalf("FetchMedia(msg-1) = %v", err)
		}
		if err := <-done2; err != nil {
			t.Fatalf("FetchMedia(msg-2) = %v", err)
		}

		ref1, _, err := media.get(t.Context(), directChat.RemoteID, "msg-1")
		if err != nil {
			t.Fatal(err)
		}
		ref2, _, err := media.get(t.Context(), directChat.RemoteID, "msg-2")
		if err != nil {
			t.Fatal(err)
		}
		if ref1.DirectPath != "/new-1" || ref2.DirectPath != "/new-2" {
			t.Errorf("stored refs = %q, %q, want /new-1 and /new-2 matched to their own message", ref1.DirectPath, ref2.DirectPath)
		}
	})
}

// TestRegisterRetryWaiter_CleanupStopsFurtherDelivery confirms the
// cleanup func registerRetryWaiter returns actually removes the
// waiter, so a notification arriving after a caller has stopped
// waiting is dropped instead of leaking into some later, unrelated
// wait for the same message id.
func TestRegisterRetryWaiter_CleanupStopsFurtherDelivery(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	c := connectedTo(dev, &connectortest.Sink{})

	ch, cleanup := c.registerRetryWaiter("msg-1")
	cleanup()

	c.deliverRetry(&events.MediaRetry{MessageID: "msg-1"})

	select {
	case got := <-ch:
		t.Errorf("delivered %v after cleanup, want nothing", got)
	default:
	}
}

// TestDeliverRetry_DropsAnAnswerNobodyIsWaitingFor confirms an answer
// for a message id this run never registered a waiter for, or already
// stopped waiting on, is silently dropped rather than panicking or
// blocking.
func TestDeliverRetry_DropsAnAnswerNobodyIsWaitingFor(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	c := connectedTo(dev, &connectortest.Sink{})

	c.deliverRetry(&events.MediaRetry{MessageID: "ghost"})
}

// TestFetchMedia_DoesNotRetryANon404Or410Failure confirms any other
// download failure, such as a different HTTP status, is returned as
// its plain self, with no retry receipt ever sent.
func TestFetchMedia_DoesNotRetryANon404Or410Failure(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.downloadErr = whatsmeow.ErrMediaDownloadFailedWith403
	var sink connectortest.Sink
	c := connectedToWithMedia(t, dev, &sink)
	if err := c.mediaFor().put(t.Context(), directChat.RemoteID, "msg-1", savedRef()); err != nil {
		t.Fatal(err)
	}

	err := c.FetchMedia(t.Context(), directChat, "msg-1", filepath.Join(t.TempDir(), "x"))
	if !errors.Is(err, dev.downloadErr) {
		t.Errorf("FetchMedia = %v, want the plain download error returned unchanged", err)
	}
	if len(dev.mediaRetryCalls) != 0 {
		t.Errorf("mediaRetryCalls = %d, want no retry receipt sent for a non-404/410 failure", len(dev.mediaRetryCalls))
	}
}
