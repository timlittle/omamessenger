package whatsapp

// fetch_image_stream.go streams a photo, sticker or GIF document to a
// temporary file, capped at whichever limit fetch.go's fetchImage
// picked (see connector.ImageFetchCap), the same way fetch_stream.go
// streams a video, file or voice note to its own, larger caps: never
// holding the whole download in memory before it is capped, however
// large a sender's reply turns out to be (see docs/decisions.md).
// Kept apart from fetch.go, which only decides whether a reference is
// an image and sanitizes it once downloaded, never writes a raw file
// itself.

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// imageDownloadSuffix names the temporary file fetchImage streams a
// photo, sticker or GIF document's ciphertext into before sanitizing
// it from disk, so the final destination path is only ever written
// once a complete, bounded download has succeeded.
const imageDownloadSuffix = ".download"

// imageStream bundles what streamImageOrRetry and
// retryStreamedImageDownload need about one image download beyond the
// original fetchRequest - where to stream it and its own byte cap -
// keeping both functions within the project's own argument limit
// rather than passing tmpPath and max as separate parameters.
type imageStream struct {
	req     fetchRequest
	tmpPath string
	max     int64
}

// streamImageOrRetry streams want.req.ref to want.tmpPath once, capped
// at want.max, and, on the same expired-link or decrypt failure
// fetch_stream.go's own streamOrRetry reacts to, asks the primary
// phone to re-upload the media and streams the fresh answer once more
// (see retry.go's retryStreamedImageDownload).
func (c *Connector) streamImageOrRetry(ctx context.Context, dev device, media *mediaStore, want imageStream) error {
	err := streamImageOnce(ctx, dev, want.req.ref, want.tmpPath, want.max)
	if err == nil {
		return nil
	}

	if recoverableStaleDigest(err, fileHasBytes(want.tmpPath)) {
		logStaleDigestAccepted()
		return nil
	}

	logDownloadFailed(err)
	if !isExpiredDownload(err) && !isDecryptFailure(err) {
		return classifyDownloadErr(err)
	}

	return c.retryStreamedImageDownload(ctx, dev, media, want, err)
}

// streamImageOnce runs one bounded, flat-deadline streamed download
// attempt for ref, writing straight to path: images are always small
// enough, once capped by max, that the idle-timeout scheme
// fetch_stream.go's own streamOnce uses for a much larger video or
// file is never needed here.
func streamImageOnce(ctx context.Context, dev device, ref mediaRef, path string, max int64) error {
	downloadCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("whatsapp: fetch media: %w", err)
	}

	bf := &boundedFile{File: f, max: max}
	downloadErr := dev.downloadMediaToFile(downloadCtx, ref, bf)

	return errors.Join(downloadErr, f.Close())
}
