package app_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"math/rand"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// writeTestPhoto writes a file large enough to make several of them
// overrun the media cache's test limit: a real, valid PNG (unlike a
// bare magic header followed by zeroes) since Send now runs every
// photo attachment through imagesafe's own sanitizer, which refuses
// anything that is not actually well-formed. Its pixels are noise
// rather than a solid colour so PNG's own compression cannot shrink
// the file far below size, keeping this helper's byte-budget
// reasoning in the tests that use it accurate.
func writeTestPhoto(t *testing.T, name string, size int) string {
	t.Helper()

	side := int(math.Sqrt(float64(size) / 4))
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	rnd := rand.New(rand.NewSource(1))
	rnd.Read(img.Pix)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return writeTestFile(t, name, buf.Bytes())
}

// TestOutgoingStatus_KeepsTheCopyUntilConfirmed confirms a pending
// message's outgoing attachment is left alone: only a confirmed
// delivery state ever retires it.
func TestOutgoingStatus_KeepsTheCopyUntilConfirmed(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 1, 1)

	pending, err := f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path})
	if err != nil || pending.Status != domain.StatusPending {
		t.Fatalf("Send() = %+v, %v", pending, err)
	}

	if _, err := os.Stat(f.outgoing.Path(pending.ID, pending.Media.FileName)); err != nil {
		t.Errorf("outgoing copy missing while still pending: %v", err)
	}
}

// TestOutgoingStatus_KeepsTheCopyWhenDeliveryFails confirms a failed
// message's outgoing attachment survives, ready for Retry.
func TestOutgoingStatus_KeepsTheCopyWhenDeliveryFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 1, 1)
	f.dispatcher.err = errors.New("offline")

	failed, err := f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path})
	if err != nil || failed.Status != domain.StatusFailed {
		t.Fatalf("Send() = %+v, %v", failed, err)
	}

	if _, err := os.Stat(f.outgoing.Path(failed.ID, failed.Media.FileName)); err != nil {
		t.Errorf("outgoing copy missing after a failed send: %v", err)
	}
}

// TestOutgoingStatus_RemovesTheOutgoingCopyOnceConfirmedSent confirms a
// file attachment's outgoing copy, no longer needed for a retry once
// delivery is confirmed, is removed.
func TestOutgoingStatus_RemovesTheOutgoingCopyOnceConfirmedSent(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestFile(t, "notes.txt", []byte("plain text file"))

	sent, err := f.commands.Send(ctx, "chat", "", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	outgoingPath := f.outgoing.Path(sent.ID, sent.Media.FileName)
	if _, err := os.Stat(outgoingPath); err != nil {
		t.Fatalf("outgoing copy missing before confirmation: %v", err)
	}

	f.ingest.OutgoingStatus(ctx, sent.ID, "remote-1", domain.StatusSent)

	if _, err := os.Stat(outgoingPath); !os.IsNotExist(err) {
		t.Errorf("outgoing copy after confirmed sent = %v, want it removed", err)
	}
}

// TestOutgoingStatus_MovesASentPhotoIntoTheCacheSoItStillDisplays
// confirms that once a sent photo's delivery is confirmed, FetchMedia
// still serves it - from the downloaded-media cache now, not a fresh
// download - instead of the outgoing copy that was just removed.
func TestOutgoingStatus_MovesASentPhotoIntoTheCacheSoItStillDisplays(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 2, 2)

	sent, err := f.commands.Send(ctx, "chat", "look", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	f.ingest.OutgoingStatus(ctx, sent.ID, "remote-1", domain.StatusSent)

	if _, err := os.Stat(f.outgoing.Path(sent.ID, sent.Media.FileName)); !os.IsNotExist(err) {
		t.Fatalf("outgoing copy after confirmed sent = %v, want it removed", err)
	}

	got, err := f.commands.FetchMedia(ctx, sent.ID, false)
	if err != nil {
		t.Fatalf("FetchMedia() error = %v", err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("FetchMedia path %q does not exist: %v", got, err)
	}
	if len(f.media.fetched) != 0 {
		t.Errorf("FetchMedia downloaded from the service: %v, want it served from the cache", f.media.fetched)
	}
}

// TestOutgoingStatus_KeepsEveryPhotoRecoverableWhenConfirmedInOrder
// sends several photos in a row, each confirmed delivered right after
// it is sent (the ordinary case), and confirms every one still
// resolves to a full image: the cache can only hold two of the three,
// so the first one sent and confirmed - the only one truly least
// recently used - is evicted and re-downloaded, never one of the
// others.
func TestOutgoingStatus_KeepsEveryPhotoRecoverableWhenConfirmedInOrder(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	const photoSize = 500 * 1024 // two fit in the test fixture's 1 MiB media cache, three do not
	var sent [3]domain.Message
	for i := range sent {
		path := writeTestPhoto(t, fmt.Sprintf("photo%d.png", i), photoSize)

		m, err := f.commands.Send(ctx, "chat", "", app.SendOptions{AttachmentPath: path})
		if err != nil {
			t.Fatalf("Send(%d) error = %v", i, err)
		}
		sent[i] = m

		f.ingest.OutgoingStatus(ctx, m.ID, fmt.Sprintf("remote-%d", i), domain.StatusSent)
	}

	assertEveryPhotoRecoverable(t, f, sent[:])
	if !slices.Equal(f.media.fetched, []string{"remote-0"}) {
		t.Errorf("redownloaded %v, want only the first photo sent and confirmed", f.media.fetched)
	}
}

// TestOutgoingStatus_KeepsEveryPhotoRecoverableWhenConfirmedOutOfOrder
// reproduces sending several photos in a row whose delivery receipts
// come back out of order, the way a slower upload can finish after a
// faster one sent moments later. The photo actually confirmed first -
// the one truly due for eviction - must be the one the cache drops, not
// whichever happens to carry the oldest outgoing-copy timestamp from
// when it was composed.
func TestOutgoingStatus_KeepsEveryPhotoRecoverableWhenConfirmedOutOfOrder(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)

	const photoSize = 500 * 1024 // two fit in the test fixture's 1 MiB media cache, three do not
	var sent [3]domain.Message
	for i := range sent {
		path := writeTestPhoto(t, fmt.Sprintf("photo%d.png", i), photoSize)

		m, err := f.commands.Send(ctx, "chat", "", app.SendOptions{AttachmentPath: path})
		if err != nil {
			t.Fatalf("Send(%d) error = %v", i, err)
		}
		sent[i] = m

		// The outgoing copy's timestamp says when it was composed, in
		// send order, well before any of the three attachments are
		// actually confirmed below.
		composed := time.Now().Add(-2*time.Hour + time.Duration(i)*time.Minute)
		if err := os.Chtimes(f.outgoing.Path(m.ID, m.Media.FileName), composed, composed); err != nil {
			t.Fatal(err)
		}
	}

	// Confirmations arrive out of order: the third photo's receipt
	// lands first, so it - not the first photo - is the one truly
	// least recently used once all three are adopted.
	f.ingest.OutgoingStatus(ctx, sent[2].ID, "remote-2", domain.StatusSent)
	f.ingest.OutgoingStatus(ctx, sent[1].ID, "remote-1", domain.StatusSent)
	f.ingest.OutgoingStatus(ctx, sent[0].ID, "remote-0", domain.StatusSent)

	assertEveryPhotoRecoverable(t, f, sent[:])
	if !slices.Equal(f.media.fetched, []string{"remote-2"}) {
		t.Errorf("redownloaded %v, want only the photo confirmed first (remote-2) evicted", f.media.fetched)
	}
}

// assertEveryPhotoRecoverable confirms FetchMedia still returns an
// existing local file for every message in sent, whether that is the
// cache's own adopted copy or one re-downloaded for a copy the cache
// has since dropped.
func assertEveryPhotoRecoverable(t *testing.T, f *fixture, sent []domain.Message) {
	t.Helper()

	for i, m := range sent {
		path, err := f.commands.FetchMedia(t.Context(), m.ID, false)
		if err != nil {
			t.Fatalf("FetchMedia(%d) error = %v", i, err)
		}

		if _, err := os.Stat(path); err != nil {
			t.Errorf("FetchMedia(%d) path %q does not exist: %v", i, path, err)
		}
	}
}
