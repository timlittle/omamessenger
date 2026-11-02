package app

import (
	"context"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// decodeSlots is how many of this helper's own full-size image
// re-encodes - connector.ReencodeImageFile (a display copy),
// SanitizeOutgoingImage (a pasted image) and SanitizeOutgoingImageFile
// (an attached or re-copied photo) - may run at once: one.
// imagesafe.Reencode's own decode is the single largest allocation any
// request this helper serves makes (see docs/decisions.md for the
// measured peaks), and the UI auto-fetches every photo as it scrolls
// into view, so without this, ten hostile photos in one chat would
// decode in parallel rather than one at a time. One slot, not two:
// the measured worst case at FullImage's own cap already approaches
// its own MaxDecodeBytes, and SanitizeOutgoingImage(File) share that
// same budget under imagesafe.Outgoing's own, larger pixel caps (see
// docs/decisions.md), so a second concurrent decode would roughly
// double the helper's own worst-case footprint for no benefit either
// an incoming photo's single-column message list or a one-at-a-time
// outgoing attachment ever needs.
const decodeSlots = 1

// acquireDecodeSlot reserves one of this Commands's decodeSlots,
// waiting until one is free or ctx ends first, and returns the func
// that releases it - call it exactly once. Every one of the three
// re-encode operations above shares the same slots regardless of
// which it is about to run: all three fill the same risk (see
// decodeSlots), and sizing one pool once is simpler than sizing three
// that would only ever be tuned together.
func (c *Commands) acquireDecodeSlot(ctx context.Context) (func(), error) {
	select {
	case c.decodeSlot <- struct{}{}:
		return func() { <-c.decodeSlot }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("app: waiting for an image decode slot: %w", ctx.Err())
	}
}

// reencodeImageFile wraps connector.ReencodeImageFile with this
// helper's own decode slot (see decodeSlots), dropping width and
// height: reencodeForDisplay's own callers already re-read a display
// copy's real dimensions from its written file afterward (see
// dimensionsOf), so threading them back through here would only ever
// go unused.
func (c *Commands) reencodeImageFile(ctx context.Context, srcPath, dstPath string, profile imagesafe.Profile) (imagesafe.OutputFormat, error) {
	release, err := c.acquireDecodeSlot(ctx)
	if err != nil {
		return imagesafe.FormatUnknown, err
	}
	defer release()

	format, _, _, err := connector.ReencodeImageFile(srcPath, dstPath, profile)

	return format, err
}

// sanitizeOutgoingImage wraps connector.SanitizeOutgoingImage with
// this helper's own decode slot (see decodeSlots), for PasteImage's
// own use.
func (c *Commands) sanitizeOutgoingImage(ctx context.Context, data []byte) ([]byte, error) {
	release, err := c.acquireDecodeSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	return connector.SanitizeOutgoingImage(data)
}
