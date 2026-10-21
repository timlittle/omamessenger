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

// fetchTimeout bounds how long one small, in-memory download attempt -
// a photo, sticker or real GIF file, or any automatic fetch - waits for
// WhatsApp's media servers, so a stalled download cannot leave a
// caller waiting forever. A user-requested video or file instead
// streams to disk under connector.IdleTimeout and
// connector.AbsoluteCeiling (see fetch_stream.go), since it may
// legitimately take far longer than this to arrive in full.
const fetchTimeout = 2 * time.Minute

// errMediaTooLarge reports a download refused because its declared or
// actual size exceeds the limit that applies to it (see
// connector.ErrMediaTooLarge, which this wraps so every connector's
// too-large fetch is reported under the one shared category): a photo,
// sticker or GIF document whose sender-declared FileLength already
// exceeds the image profile's own input cap, checked before any bytes
// are downloaded since whatsmeow's in-memory DownloadMediaWithPath
// returns a file in one piece with no way to cap it mid-stream; or a
// video, file or voice note whose declared or actual size exceeds
// connector.MaxBytes, checked and bounded the same way Telegram's own
// chunked downloader is (see fetch_stream.go). Either way this is a
// permanent failure, never retried.
var errMediaTooLarge = fmt.Errorf("whatsapp: %w", connector.ErrMediaTooLarge)

// fetchRequest bundles everything one FetchMedia call acts on - the
// conversation, message id, saved reference, destination path and
// whether this is an automatic fetch - so passing all of it through
// downloadOrRetry, retryDownload and the streamed path's own
// equivalents (see fetch_stream.go) does not grow any one function's
// own argument list every time another of them needs it.
type fetchRequest struct {
	conv            domain.Conversation
	messageRemoteID string
	ref             mediaRef
	path            string
	auto            bool
}

// FetchMedia downloads and decrypts the photo, video, voice note or
// file of the message conv/messageRemoteID names, writing it to path.
// A reference that was never saved is reported as domain.ErrNotFound,
// which the UI already shows as "photo unavailable"; a link WhatsApp's
// CDN no longer recognises is retried once through the primary phone
// (see retry.go) before failing the same way, as domain.ErrMediaExpired.
// auto is true for a fetch the UI made on its own, with no click from
// the user - today, only an animated "GIF" video autoplaying as soon as
// it is shown - which a video, file or voice note bounds to
// connector.AutoFetchMaxBytes rather than the far larger limit a
// deliberate click is trusted with (see connector.MaxBytes); it makes
// no difference to a photo, sticker or real GIF file, which is already
// bounded by its own, stricter imagesafe profile regardless.
func (c *Connector) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string, auto bool) error {
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

	req := fetchRequest{conv: conv, messageRemoteID: messageRemoteID, ref: ref, path: path, auto: auto}

	if _, isImage := imageProfile(ref); isImage {
		return c.fetchImage(ctx, dev, media, req)
	}

	return c.fetchFile(ctx, dev, media, req)
}

// fetchImage downloads a photo, sticker or real GIF file fully into
// memory and writes it through imagesafe's sanitizer: unaffected by
// req.auto, since these kinds are already bounded by their own,
// stricter profile (see oversizedImage).
func (c *Connector) fetchImage(ctx context.Context, dev device, media *mediaStore, req fetchRequest) error {
	if oversizedImage(req.ref) {
		return fmt.Errorf("whatsapp: fetch media: %w", errMediaTooLarge)
	}

	data, err := c.downloadOrRetry(ctx, dev, media, req)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	profile, _ := imageProfile(req.ref)
	if err := connector.SaveImageMedia(req.path, data, profile); err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	return nil
}

// oversizedImage reports whether ref is a photo, sticker or GIF
// document whose own declared FileLength is already larger than the
// cap the matching imagesafe.Profile would check once downloaded:
// checked before fetchImage asks whatsmeow to download anything at
// all, since there is no way to cap that download mid-stream (see
// errMediaTooLarge). A kind imageProfile does not cover at all, or
// one that declares no length, is never flagged here: the actual
// bytes still pass through imagesafe.Sanitize's own cap once
// downloaded, same as always.
func oversizedImage(ref mediaRef) bool {
	profile, ok := imageProfile(ref)

	return ok && ref.FileLength > 0 && int64(ref.FileLength) > profile.MaxInputLen
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

// recoverStaleDigest reports whether an in-memory download that failed
// only because the file no longer matches the plaintext hash its
// message declared is safe to use anyway: whatsmeow sets data to the
// decrypted bytes before running that specific check (see its
// downloadAndDecrypt), so by the time it fails the media-key HMAC has
// already authenticated the ciphertext against this message's own
// key. See recoverableStaleDigest for why this is safe to accept, and
// fetch_stream.go's own use of it for the streamed-to-file path, where
// the already-decrypted bytes are the file on disk rather than data.
func recoverStaleDigest(data []byte, err error) ([]byte, bool) {
	if !recoverableStaleDigest(err, len(data) > 0) {
		return nil, false
	}

	return data, true
}

// recoverableStaleDigest reports whether a download that failed with
// whatsmeow.ErrInvalidMediaSHA256 - and nothing else - still has
// already-decrypted, HMAC-authenticated bytes sitting somewhere safe to
// use: hasBytes is len(data) > 0 for the in-memory path, or the file
// written to disk being non-empty for the streamed one. WhatsApp's
// media retry can answer with a file that was re-encoded when the
// phone re-uploaded it, so its plaintext legitimately no longer
// matches the hash the original message declared; WhatsApp's own apps
// accept the file anyway, so this connector does too (see
// docs/decisions.md). Any other decrypt failure never reaches this far
// with bytes at all: the caller's own classifyDownloadErr still
// reports it.
func recoverableStaleDigest(err error, hasBytes bool) bool {
	return hasBytes && errors.Is(err, whatsmeow.ErrInvalidMediaSHA256)
}
