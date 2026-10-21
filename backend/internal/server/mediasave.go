package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// mediaSave downloads a message's media if need be, then copies it into
// the user's downloads directory under a name that never overwrites an
// existing file there.
func mediaSave(c *app.Commands) func(context.Context, retryParams) (any, error) {
	return func(ctx context.Context, p retryParams) (any, error) {
		path, err := c.SaveMedia(ctx, p.MessageID)

		return mediaResult{Path: path}, err
	}
}
