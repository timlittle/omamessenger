package whatsapp

// fetch.go downloads a message's photo, video, voice note or file: the
// mediaRef this connector saved when the message arrived (see
// normalize_media.go and storage.go) is the only way to find it again,
// since WhatsApp's end-to-end messages carry no server copy to ask a
// fresh reference from the way Telegram's FetchMedia does.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fetchTimeout bounds how long FetchMedia waits for WhatsApp's media
// servers, so a stalled download cannot leave a caller waiting forever.
const fetchTimeout = 2 * time.Minute

// FetchMedia downloads and decrypts the photo, video, voice note or
// file of the message conv/messageRemoteID names, writing it to path.
// A reference that was never saved is reported as domain.ErrNotFound,
// which the UI already shows as "photo unavailable"; a reference
// WhatsApp's media servers no longer recognise fails the same way,
// through the download error itself.
func (c *Connector) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	media := c.mediaFor()
	if media == nil {
		return errNotConnected
	}

	ref, ok, err := media.get(ctx, conv.RemoteID, messageRemoteID)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}
	if !ok {
		return fmt.Errorf("whatsapp: fetch media: %w", domain.ErrNotFound)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	data, err := dev.downloadMedia(fetchCtx, ref)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	return nil
}
