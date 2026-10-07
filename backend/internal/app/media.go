package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// FetchMedia returns the path of a message's photo, video or file,
// downloading it from the service the first time.
func (c *Commands) FetchMedia(ctx context.Context, messageID string) (string, error) {
	if strings.TrimSpace(messageID) == "" {
		return "", fmt.Errorf("%w: messageId is required", ErrInvalidInput)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return "", err
	}

	if m.Media == nil || m.Media.Kind == domain.MediaLink || m.RemoteID == "" || c.media == nil || c.cache == nil {
		return "", fmt.Errorf("%w: this message has nothing to download", ErrInvalidInput)
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return "", err
	}

	return c.cache.Fetch(ctx, mediaFileName(m), func(ctx context.Context, path string) error {
		return c.media.FetchMedia(ctx, conv, m.RemoteID, path)
	})
}

// mediaFileName names a message's media in the cache by the message's id,
// with an extension that says what it is, or for a file its own name, so
// it opens in the right application.
func mediaFileName(m domain.Message) string {
	switch m.Media.Kind {
	case domain.MediaPhoto:
		return m.ID + ".jpg"
	case domain.MediaVideo:
		return m.ID + ".mp4"
	}

	name := filepath.Base(m.Media.FileName)
	if name == "." || name == "/" {
		return m.ID
	}

	return m.ID + "-" + name
}
