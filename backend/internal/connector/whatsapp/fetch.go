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
	"time"

	"go.mau.fi/whatsmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fetchTimeout bounds how long one download attempt waits for
// WhatsApp's media servers, so a stalled download cannot leave a
// caller waiting forever.
const fetchTimeout = 2 * time.Minute

// errMediaTooLarge reports a photo, sticker or GIF document whose
// sender-declared FileLength is already larger than the image
// profile's own input cap. whatsmeow's DownloadMediaWithPath returns a
// file in one piece, with no way to cap it mid-stream the way
// Telegram's own chunked downloader can (see telegram/fetch.go), so
// this is the one check this connector can make: refused before any
// bytes are downloaded, and never retried.
var errMediaTooLarge = errors.New("whatsapp: declared media size exceeds the limit")

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

	if oversizedImage(ref) {
		return fmt.Errorf("whatsapp: fetch media: %w", errMediaTooLarge)
	}

	req := fetchRequest{conv: conv, messageRemoteID: messageRemoteID, ref: ref}
	data, err := c.downloadOrRetry(ctx, dev, media, req)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	if err := saveFetchedMedia(path, data, ref); err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	return nil
}

// oversizedImage reports whether ref is a photo, sticker or GIF
// document whose own declared FileLength is already larger than the
// cap the matching imagesafe.Profile would check once downloaded:
// checked before FetchMedia asks whatsmeow to download anything at
// all, since there is no way to cap that download mid-stream (see
// errMediaTooLarge). A kind imageProfile does not cover at all, or
// one that declares no length, is never flagged here: the actual
// bytes still pass through imagesafe.Sanitize's own cap once
// downloaded, same as always.
func oversizedImage(ref mediaRef) bool {
	profile, ok := imageProfile(ref)

	return ok && ref.FileLength > 0 && int64(ref.FileLength) > profile.MaxInputLen
}

// saveFetchedMedia writes data to path, through the sanitizer every
// photo, sticker and GIF must pass before the UI can load it into a
// QML Image or AnimatedImage, or unchanged for every other kind,
// which the UI never shows that way.
func saveFetchedMedia(path string, data []byte, ref mediaRef) error {
	if profile, ok := imageProfile(ref); ok {
		return connector.SaveImageMedia(path, data, profile)
	}

	return connector.SaveMedia(path, data)
}

// downloadOrRetry downloads ref once, and, when WhatsApp's CDN reports
// the link has expired (see isExpiredDownload) or the file no longer
// decrypts with the key or hash the message carried (see
// isDecryptFailure), asks the primary phone to re-upload the media (see
// retry.go) and tries the download once more with whatever new path it
// answers with: a reference that looks permanently stale can still be
// one WhatsApp's primary device re-encrypted on its last upload, which
// only a fresh retry answer, not a second attempt at the same link, can
// recover from. Any other failure is returned as-is, with
// domain.ErrMediaDecryptFailed joined in for a hash or HMAC mismatch.
func (c *Connector) downloadOrRetry(ctx context.Context, dev device, media *mediaStore, req fetchRequest) ([]byte, error) {
	data, err := downloadOnce(ctx, dev, req.ref)
	if err == nil {
		return data, nil
	}

	if recovered, ok := recoverStaleDigest(data, err); ok {
		logStaleDigestAccepted()
		return recovered, nil
	}

	logDownloadFailed(err)
	if !isExpiredDownload(err) && !isDecryptFailure(err) {
		return nil, classifyDownloadErr(err)
	}

	return c.retryDownload(ctx, dev, media, req, err)
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

// recoverStaleDigest reports whether a download that failed only
// because the file no longer matches the plaintext hash its message
// declared is safe to use anyway: whatsmeow sets data to the decrypted
// bytes before running that specific check (see its downloadAndDecrypt),
// so by the time it fails the media-key HMAC has already authenticated
// the ciphertext against this message's own key. WhatsApp's media retry
// can answer with a file that was re-encoded when the phone re-uploaded
// it, so its plaintext legitimately no longer matches the hash the
// original message declared; WhatsApp's own apps accept the file
// anyway, so this connector does too (see docs/decisions.md). Any other
// decrypt failure never reaches this far with data at all: ok is false,
// and the caller's own classifyDownloadErr still reports it.
func recoverStaleDigest(data []byte, err error) ([]byte, bool) {
	if len(data) == 0 || !errors.Is(err, whatsmeow.ErrInvalidMediaSHA256) {
		return nil, false
	}

	return data, true
}
