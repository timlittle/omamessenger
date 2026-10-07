package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// FetchMedia returns the path of a message's photo, video or file. An
// outgoing message's own attachment is already on this machine, so its
// local copy is returned at once, with no remote id needed; anything
// else is downloaded from the service the first time, then kept in the
// media cache.
func (c *Commands) FetchMedia(ctx context.Context, messageID string) (string, error) {
	if strings.TrimSpace(messageID) == "" {
		return "", fmt.Errorf("%w: messageId is required", ErrInvalidInput)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return "", err
	}

	if m.Media == nil || m.Media.Kind == domain.MediaLink {
		return "", fmt.Errorf("%w: this message has nothing to download", ErrInvalidInput)
	}

	if path, ok := c.localAttachment(m); ok {
		return path, nil
	}

	if m.RemoteID == "" || c.media == nil || c.cache == nil {
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

// localAttachment returns the path of an outgoing message's own
// attachment, already copied into the outgoing media area when it was
// sent, so the UI never has to download a photo back from the service it
// was just uploaded to. It reports false for an incoming message, or an
// outgoing one whose local copy is gone, so the caller falls back to
// downloading it.
func (c *Commands) localAttachment(m domain.Message) (string, bool) {
	if !m.Outgoing || c.outgoing == nil {
		return "", false
	}

	path := c.outgoing.Path(m.ID, m.Media.FileName)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}

	return path, true
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
