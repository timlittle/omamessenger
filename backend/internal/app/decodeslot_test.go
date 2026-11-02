// decodeslot_test.go is an internal test (package app, not app_test):
// connector.ReencodeImageFile, SanitizeOutgoingImage and
// SanitizeOutgoingImageFile have no interface seam to inject a
// controllable fake through, so there is no way to drive
// acquireDecodeSlot's own blocking and release behaviour deterministically
// through the public API without a real, pointlessly slow image
// decode. This tests acquireDecodeSlot directly instead - the one
// unexported mechanism that actually does the serializing, independent
// of what it protects.
package app

import (
	"context"
	"testing"
	"time"
)

// TestAcquireDecodeSlot_SerializesCallers covers the property
// decodeSlots exists to guarantee: with one slot, a second acquire
// blocks while the first is still held, and only succeeds once the
// first releases it.
func TestAcquireDecodeSlot_SerializesCallers(t *testing.T) {
	t.Parallel()

	c := &Commands{decodeSlot: make(chan struct{}, 1)}
	ctx := t.Context()

	release, err := c.acquireDecodeSlot(ctx)
	if err != nil {
		t.Fatalf("first acquireDecodeSlot() error = %v", err)
	}

	blockedCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	if _, err := c.acquireDecodeSlot(blockedCtx); err == nil {
		t.Fatal("second acquireDecodeSlot() succeeded while the first still held its one slot, want it blocked until ctx ends")
	}

	release()

	release2, err := c.acquireDecodeSlot(ctx)
	if err != nil {
		t.Fatalf("acquireDecodeSlot() after release = %v, want nil: the slot should be free again", err)
	}
	release2()
}

// TestAcquireDecodeSlot_RefusesAnAlreadyCancelledContext covers the
// other way a caller never gets a slot: ctx already done before it
// even tries. The one slot is held first, so acquireDecodeSlot's own
// select has only its ctx.Done() case ready; with the slot free
// instead, a send to its buffered channel is also always ready, and
// Go's select picks uniformly at random between two ready cases - a
// real flake this test hit once, not a hypothetical one.
func TestAcquireDecodeSlot_RefusesAnAlreadyCancelledContext(t *testing.T) {
	t.Parallel()

	c := &Commands{decodeSlot: make(chan struct{}, 1)}
	release, err := c.acquireDecodeSlot(t.Context())
	if err != nil {
		t.Fatalf("acquireDecodeSlot() to hold the slot = %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := c.acquireDecodeSlot(ctx); err == nil {
		t.Fatal("acquireDecodeSlot() with an already-cancelled context succeeded, want an error")
	}
}
