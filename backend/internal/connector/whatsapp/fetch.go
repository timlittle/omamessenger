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

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fetchTimeout bounds how long one small, streamed download attempt -
// a photo, sticker or real GIF file, capped by imageFetchCap, or any
// automatic fetch - waits for WhatsApp's media servers, so a stalled
// download cannot leave a caller waiting forever. A user-requested
// video or file instead streams to disk under connector.IdleTimeout
// and connector.AbsoluteCeiling (see fetch_stream.go), since it may
// legitimately take far longer than this to arrive in full.
const fetchTimeout = 2 * time.Minute

// errMediaTooLarge reports a download refused because its declared or
// actual size exceeds the limit that applies to it (see
// connector.ErrMediaTooLarge, which this wraps so every connector's
// too-large fetch is reported under the one shared category): a photo,
// sticker or GIF document whose declared or actual size exceeds
// imageFetchCap's own limit, or a video, file or voice note whose
// declared or actual size exceeds connector.MaxBytes - both streamed
// to disk and capped mid-transfer the same way, never trusting a
// sender's declared size as the only gate (see fetch_stream.go and
// docs/decisions.md). Either way this is a permanent failure, never
// retried.
var errMediaTooLarge = fmt.Errorf("whatsapp: %w", connector.ErrMediaTooLarge)

// fetchRequest bundles everything one FetchMedia call acts on - the
// conversation, message id, saved reference, destination path and
// whether this is an automatic fetch - so passing all of it through
// streamImageOrRetry, retryStreamedImageDownload and the non-image
// streamed path's own equivalents (see fetch_stream.go) does not grow
// any one function's own argument list every time another of them
// needs it.
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
// the user - a photo, sticker or voice note shown as a message's row
// comes into view, or an animated "GIF" video autoplaying - which
// bounds the download to connector.AutoFetchMaxBytes rather than the
// far larger limit a deliberate click is trusted with (see
// connector.MaxBytes and imageFetchCap).
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

// fetchImage downloads a photo, sticker or real GIF file to a
// temporary file next to req.path, streamed and capped the same way
// fetch_stream.go bounds a video or file, then sanitizes it from
// disk: a security review found the previous, in-memory download
// trusted the sender's own declared FileLength as its only gate,
// skipping the check entirely when that field was left at 0, and only
// then read the whole reply into memory with no cap of its own at
// all. Capped at the image profile's own MaxInputLen, and narrowed
// further to connector.AutoFetchMaxBytes when req.auto is set (see
// connector.ImageFetchCap), so an automatic fetch of a photo or
// sticker is never trusted with more than any other automatic fetch
// is.
func (c *Connector) fetchImage(ctx context.Context, dev device, media *mediaStore, req fetchRequest) error {
	profile, _ := imageProfile(req.ref)
	max := connector.ImageFetchCap(profile, req.auto)
	if req.ref.FileLength > 0 && int64(req.ref.FileLength) > max {
		return fmt.Errorf("whatsapp: fetch media: %w", errMediaTooLarge)
	}

	want := imageStream{req: req, tmpPath: req.path + imageDownloadSuffix, max: max}
	defer removeIfPresent(want.tmpPath)
	if err := c.streamImageOrRetry(ctx, dev, media, want); err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	data, err := os.ReadFile(want.tmpPath)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	if err := connector.SaveImageMedia(req.path, data, profile); err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	return nil
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

// recoverableStaleDigest reports whether a download that failed with
// whatsmeow.ErrInvalidMediaSHA256 - and nothing else - still has
// already-decrypted, HMAC-authenticated bytes sitting somewhere safe to
// use: hasBytes is the file written to disk being non-empty, for every
// kind this connector downloads - a photo, sticker or GIF document
// streamed by fetchImage, or a video, file or voice note streamed by
// fetchFile (see fetch_stream.go). whatsmeow writes the decrypted bytes
// to that file before running the plaintext-hash check that can fail
// this way, so by the time it fails the media-key HMAC has already
// authenticated the ciphertext against this message's own key.
// WhatsApp's media retry can answer with a file that was re-encoded when the
// phone re-uploaded it, so its plaintext legitimately no longer
// matches the hash the original message declared; WhatsApp's own apps
// accept the file anyway, so this connector does too (see
// docs/decisions.md). Any other decrypt failure never reaches this far
// with bytes at all: the caller's own classifyDownloadErr still
// reports it.
func recoverableStaleDigest(err error, hasBytes bool) bool {
	return hasBytes && errors.Is(err, whatsmeow.ErrInvalidMediaSHA256)
}
