package whatsapp

// fetch.go downloads a message's photo, video, voice note or file: the
// mediaRef this connector saved when the message arrived (see
// normalize_media.go and storage.go) is the only way to find it again,
// since WhatsApp's end-to-end messages carry no server copy to ask a
// fresh reference from the way Telegram's FetchMedia does. When
// WhatsApp's CDN reports that reference's link has expired (a 404 or
// 410), retry.go asks the primary phone to re-upload the media before
// this file gives up.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.mau.fi/whatsmeow"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fetchTimeout bounds how long one download attempt waits for
// WhatsApp's media servers, so a stalled download cannot leave a
// caller waiting forever.
const fetchTimeout = 2 * time.Minute

// fetchRequest bundles the conversation, message id and saved
// reference one FetchMedia call acts on, so passing all three through
// downloadOrRetry and retryDownload does not grow either function's
// own argument list every time one more of them is needed.
type fetchRequest struct {
	conv            domain.Conversation
	messageRemoteID string
	ref             mediaRef
}

// FetchMedia downloads and decrypts the photo, video, voice note or
// file of the message conv/messageRemoteID names, writing it to path.
// A reference that was never saved is reported as domain.ErrNotFound,
// which the UI already shows as "photo unavailable"; a link WhatsApp's
// CDN no longer recognises is retried once through the primary phone
// (see retry.go) before failing the same way, as domain.ErrMediaExpired.
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

	req := fetchRequest{conv: conv, messageRemoteID: messageRemoteID, ref: ref}
	data, err := c.downloadOrRetry(ctx, dev, media, req)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	return nil
}

// downloadOrRetry downloads ref once, and, when WhatsApp's CDN reports
// the link has expired (see isExpiredDownload), asks the primary phone
// to re-upload the media (see retry.go) and tries the download once
// more with whatever new path it answers with. Any other failure is
// returned as-is, with domain.ErrMediaDecryptFailed joined in for a
// hash or HMAC mismatch.
func (c *Connector) downloadOrRetry(ctx context.Context, dev device, media *mediaStore, req fetchRequest) ([]byte, error) {
	data, err := downloadOnce(ctx, dev, req.ref)
	if err == nil {
		return data, nil
	}

	logDownloadFailed(downloadFailureClass(err))
	if !isExpiredDownload(err) {
		return nil, classifyDownloadErr(err)
	}

	return c.retryDownload(ctx, dev, media, req)
}

// downloadOnce runs one bounded download attempt.
func downloadOnce(ctx context.Context, dev device, ref mediaRef) ([]byte, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	return dev.downloadMedia(fetchCtx, ref)
}

// decryptErrs are whatsmeow's own errors for a downloaded file that does
// not verify against the key or hash the message carried: a worn-out or
// tampered-with reference, as opposed to the server simply being
// unreachable. classifyDownloadErr tells the two apart so the app
// layer's "decrypt" reason is only ever reported for these.
var decryptErrs = []error{
	whatsmeow.ErrInvalidMediaHMAC,
	whatsmeow.ErrInvalidMediaEncSHA256,
	whatsmeow.ErrInvalidMediaSHA256,
	whatsmeow.ErrInvalidUnencryptedMediaSHA256,
}

// isDecryptFailure reports whether err is one of decryptErrs.
func isDecryptFailure(err error) bool {
	for _, want := range decryptErrs {
		if errors.Is(err, want) {
			return true
		}
	}

	return false
}

// classifyDownloadErr joins domain.ErrMediaDecryptFailed into err when
// whatsmeow reports one of decryptErrs, so errors.Is(err,
// domain.ErrMediaDecryptFailed) finds it further up the call chain; any
// other error, including a plain network or server failure, is returned
// unchanged.
func classifyDownloadErr(err error) error {
	if isDecryptFailure(err) {
		return errors.Join(domain.ErrMediaDecryptFailed, err)
	}

	return err
}
