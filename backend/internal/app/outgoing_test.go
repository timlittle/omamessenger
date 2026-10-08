package app_test

import (
	"errors"
	"os"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestOutgoingStatus_KeepsTheCopyUntilConfirmed confirms a pending
// message's outgoing attachment is left alone: only a confirmed
// delivery state ever retires it.
func TestOutgoingStatus_KeepsTheCopyUntilConfirmed(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	path := writeTestPNG(t, "photo.png", 1, 1)

	pending, err := f.commands.Send(ctx, "chat", "hi", path, "")
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

	failed, err := f.commands.Send(ctx, "chat", "hi", path, "")
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

	sent, err := f.commands.Send(ctx, "chat", "", path, "")
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

	sent, err := f.commands.Send(ctx, "chat", "look", path, "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	f.ingest.OutgoingStatus(ctx, sent.ID, "remote-1", domain.StatusSent)

	if _, err := os.Stat(f.outgoing.Path(sent.ID, sent.Media.FileName)); !os.IsNotExist(err) {
		t.Fatalf("outgoing copy after confirmed sent = %v, want it removed", err)
	}

	got, err := f.commands.FetchMedia(ctx, sent.ID)
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
