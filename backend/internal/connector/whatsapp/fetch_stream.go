package whatsapp

// fetch_stream.go streams a video, file or voice note straight to
// disk, bounded by the limit auto selects and capped at its own
// declared size plus a small tolerance, never holding the whole thing
// in memory at once (see download_file.go and docs/decisions.md).
// fetch.go's own fetchImage streams a photo, sticker or real GIF file
// the same way, capped at the image profile's own limit instead. The
// retry-via-primary-phone behaviour here is the same shape as
// fetch.go's streamImageOrRetry, just over this package's own larger
// size caps: see retry.go's retryStreamedDownload.

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// fetchFile streams req.ref to req.path, refusing outright a declared
// size already over the limit req.auto selects, and otherwise bounding
// the bytes actually received the same way (see connector.MaxBytes and
// connector.ByteCap).
func (c *Connector) fetchFile(ctx context.Context, dev device, media *mediaStore, req fetchRequest) error {
	if req.ref.FileLength > 0 && int64(req.ref.FileLength) > connector.MaxBytes(req.auto) {
		return fmt.Errorf("whatsapp: fetch media: %w", errMediaTooLarge)
	}

	if err := c.streamOrRetry(ctx, dev, media, req); err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	return nil
}

// streamOrRetry streams req.ref to req.path once, and, on the same
// expired-link or decrypt failure fetchImage's own downloadOrRetry
// reacts to, asks the primary phone to re-upload the media and streams
// the fresh answer once more (see retry.go's retryStreamedDownload). A
// failure that leaves a stale-digest-recoverable file on disk is kept
// rather than removed (see recoverableStaleDigest); any other failure
// removes whatever partial bytes streamOnce already wrote, best
// effort, since the error being returned is what actually matters to
// the caller.
func (c *Connector) streamOrRetry(ctx context.Context, dev device, media *mediaStore, req fetchRequest) error {
	err := streamOnce(ctx, dev, req.ref, req.path, req.auto)
	if err == nil {
		return nil
	}

	if recoverableStaleDigest(err, fileHasBytes(req.path)) {
		logStaleDigestAccepted()
		return nil
	}

	removeIfPresent(req.path)
	logDownloadFailed(err)
	if !isExpiredDownload(err) && !isDecryptFailure(err) {
		return classifyDownloadErr(err)
	}

	return c.retryStreamedDownload(ctx, dev, media, req, err)
}

// streamOnce runs one bounded, deadline-aware streamed download
// attempt for ref, writing straight to path; auto picks the limit and
// deadline strategy (see connector.MaxBytes and downloadDeadline).
func streamOnce(ctx context.Context, dev device, ref mediaRef, path string, auto bool) error {
	downloadCtx, onWrite, stop := downloadDeadline(ctx, auto)
	defer stop()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	max := connector.ByteCap(int64(ref.FileLength), connector.MaxBytes(auto))
	bf := &boundedFile{File: f, max: max, onWrite: onWrite}
	downloadErr := dev.downloadMediaToFile(downloadCtx, ref, bf)

	return errors.Join(downloadErr, f.Close())
}

// downloadDeadline bounds one streamed download attempt: the same
// flat fetchTimeout a small, automatic fetch already used before
// streaming existed, since AutoFetchMaxBytes keeps those quick
// regardless, or an idle timeout with a longer absolute ceiling for a
// user-requested file large enough to need one - a slow but steadily
// arriving transfer should not be cut off just because it takes
// longer than a small file would, while a connection that has
// actually stalled is still caught quickly (see connector.IdleContext).
func downloadDeadline(ctx context.Context, auto bool) (downloadCtx context.Context, onWrite func(), stop func()) {
	if auto {
		downloadCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		return downloadCtx, func() {}, cancel
	}

	return connector.IdleContext(ctx, connector.IdleTimeout, connector.AbsoluteCeiling)
}

// fileHasBytes reports whether path exists and is non-empty, standing
// in for the in-memory path's own len(data) > 0 check (see
// recoverableStaleDigest) once a download writes straight to disk.
func fileHasBytes(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Size() > 0
}

// removeIfPresent removes path, best effort: there is nothing more
// useful a caller already reporting its own download error can do if
// this fails too.
func removeIfPresent(path string) {
	_ = os.Remove(path) // best effort cleanup; the caller's own download error is what matters
}
