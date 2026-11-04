package app

import (
	"context"
	"fmt"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// decodeSlots is how many of this helper's full-size image re-encodes
// may run at once: one, since imagesafe.Reencode's decode is the
// single largest allocation this helper makes (see docs/decisions.md),
// and the UI auto-fetches every photo as it scrolls into view.
const decodeSlots = 1

// acquireDecodeSlot reserves one of this Commands's decodeSlots,
// waiting until one is free or ctx ends first, and returns the func
// that releases it - call it exactly once. All re-encode operations
// share the same pool, since they share the same risk (see decodeSlots).
func (c *Commands) acquireDecodeSlot(ctx context.Context) (func(), error) {
	select {
	case c.decodeSlot <- struct{}{}:
		return func() { <-c.decodeSlot }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("app: waiting for an image decode slot: %w", ctx.Err())
	}
}

// reencodeImageFile wraps connector.ReencodeImageFile with this
// helper's decode slot (see decodeSlots), dropping width and height:
// callers re-read a display copy's real dimensions afterward (see dimensionsOf).
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
