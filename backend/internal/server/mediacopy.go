package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// mediaCopy downloads a message's media if need be, then puts it on
// the system clipboard: the method a photo, sticker or video's own
// Copy action now calls instead of the QML side shelling out to
// wl-copy itself (see docs/decisions.md). There is nothing to report
// back beyond success, so the result is empty.
func mediaCopy(c *app.Commands) func(context.Context, retryParams) (any, error) {
	return func(ctx context.Context, p retryParams) (any, error) {
		return struct{}{}, c.CopyMedia(ctx, p.MessageID)
	}
}
