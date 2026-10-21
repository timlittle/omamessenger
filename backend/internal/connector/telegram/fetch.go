package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// errNoFile reports a message with no photo or file to download.
var errNoFile = errors.New("telegram: the message has nothing to download")

// errMediaTooLarge reports a photo, sticker or GIF whose declared or
// actual size is larger than the image profile it must pass: refused
// outright before FetchMedia asks Telegram for anything, or, for a
// stream whose declared size undersold what Telegram's CDN actually
// sends, as soon as downloadImage notices. Either way this is a
// permanent failure, never retried. It wraps connector.ErrMediaTooLarge
// so a video or other document refused the same way for the same
// reason (see download) is reported under the one shared category.
var errMediaTooLarge = fmt.Errorf("telegram: %w", connector.ErrMediaTooLarge)

// fetchTimeout bounds how long one image download attempt waits for
// Telegram's servers, so a stalled download cannot leave a caller
// waiting forever.
const fetchTimeout = 2 * time.Minute

// FetchMedia downloads the photo or file of the message Telegram numbers
// messageRemoteID to path. A message's file reference expires, so the
// message is read again for a fresh one first. auto is true for a
// fetch the UI made on its own, with no click from the user - a
// photo, sticker or voice note shown as a message's row comes into
// view, or an animated "GIF" video autoplaying - which bounds the
// download to connector.AutoFetchMaxBytes rather than the far larger
// limit a deliberate click is trusted with (see connector.MaxBytes
// and connector.ImageFetchCap).
func (c *Connector) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string, auto bool) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	id, err := strconv.Atoi(messageRemoteID)
	if err != nil {
		return fmt.Errorf("telegram: fetch media: %w", err)
	}

	msg, err := reread(ctx, api, conv, id)
	if err != nil {
		return err
	}

	location, err := fileLocation(msg.Media)
	if err != nil {
		return err
	}

	declared, known := declaredMediaSize(msg.Media)

	profile, ok := imageProfile(msg.Media)
	if !ok {
		return download(ctx, api, fileDownload{location: location, path: path, auto: auto, declaredSize: declared})
	}

	max := connector.ImageFetchCap(profile, auto)
	if known && declared > max {
		return errMediaTooLarge
	}

	return downloadImage(ctx, api, imageDownload{location: location, path: path, profile: profile, max: max})
}

// declaredMediaSize is the size m's sender claims for the file
// FetchMedia is about to download - a photo's largest size, or a
// document's own Size field - and whether m carries one at all:
// false only for a media kind imageProfile already filtered out
// before this is ever called.
func declaredMediaSize(m tg.MessageMediaClass) (int64, bool) {
	switch mm := m.(type) {
	case *tg.MessageMediaPhoto:
		if photo, ok := mm.Photo.(*tg.Photo); ok {
			_, _, _, size := largest(photo.Sizes)
			return size, true
		}
	case *tg.MessageMediaDocument:
		if doc, ok := mm.Document.(*tg.Document); ok {
			return doc.Size, true
		}
	}

	return 0, false
}

// imageDownload bundles what downloadImage needs about a photo,
// sticker or GIF document's own download, keeping that function
// within the project's own argument limit: where Telegram keeps it,
// where to write it, the imagesafe.Profile its bytes must pass and
// its own byte cap (see connector.ImageFetchCap).
type imageDownload struct {
	location tg.InputFileLocationClass
	path     string
	profile  imagesafe.Profile
	max      int64
}

// downloadImage downloads id.location into memory, bounded by id.max
// (see connector.ImageFetchCap) even if Telegram's declared size
// undersold what it actually streams (see boundedBuffer, the backstop
// under FetchMedia's own declaredMediaSize check), and writes the
// result to id.path through the sanitizer every photo, sticker or GIF
// must pass before the UI can load it into a QML Image (see
// connector.SaveImageMedia). A video or other document instead streams
// straight to disk through download below: formats Qt's image
// decoders never touch, so buffering them in memory first would cost
// without ever paying for anything.
func downloadImage(ctx context.Context, api *tg.Client, id imageDownload) error {
	downloadCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	buf := &boundedBuffer{max: id.max}
	if _, err := downloader.NewDownloader().Download(api, id.location).Stream(downloadCtx, buf); err != nil {
		if errors.Is(err, errMediaTooLarge) {
			return errMediaTooLarge
		}

		return fmt.Errorf("telegram: fetch media: %w", err)
	}

	if err := connector.SaveImageMedia(id.path, buf.Bytes(), id.profile); err != nil {
		return fmt.Errorf("telegram: fetch media: %w", err)
	}

	return nil
}

// boundedBuffer collects a download's bytes the same way bytes.Buffer
// does, but fails once more than max have arrived: the last line of
// defence against a stream larger than its own declared size, so this
// never holds more than one profile's worth of bytes in memory however
// much Telegram's CDN actually sends.
type boundedBuffer struct {
	buf bytes.Buffer
	max int64
}

// Write appends p, refusing once the total written would exceed max bytes.
func (b *boundedBuffer) Write(p []byte) (int, error) {
	if int64(b.buf.Len())+int64(len(p)) > b.max {
		return 0, errMediaTooLarge
	}

	return b.buf.Write(p)
}

// Bytes returns everything written so far.
func (b *boundedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

// reread fetches one message again, from a channel or a chat.
func reread(ctx context.Context, api *tg.Client, conv domain.Conversation, id int) (*tg.Message, error) {
	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return nil, err
	}

	result, err := fetchMessages(ctx, api, peer, []tg.InputMessageClass{&tg.InputMessageID{ID: id}})
	if err != nil {
		return nil, fmt.Errorf("telegram: fetch media: %w", err)
	}

	if messages, ok := result.AsModified(); ok {
		for _, m := range messages.GetMessages() {
			if msg, ok := m.(*tg.Message); ok && msg.ID == id {
				return msg, nil
			}
		}
	}

	return nil, fmt.Errorf("telegram: fetch media: %w", domain.ErrNotFound)
}

// fetchMessages asks Telegram for the messages named by want, from a
// channel or an ordinary chat or user.
func fetchMessages(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, want []tg.InputMessageClass) (tg.MessagesMessagesClass, error) {
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		channel := &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash}
		return api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{Channel: channel, ID: want})
	}

	return api.MessagesGetMessages(ctx, want)
}

// fileLocation is where Telegram keeps a message's photo, at its largest
// size, or its file.
func fileLocation(m tg.MessageMediaClass) (tg.InputFileLocationClass, error) {
	switch m := m.(type) {
	case *tg.MessageMediaPhoto:
		if photo, ok := m.Photo.(*tg.Photo); ok {
			kind, _, _, _ := largest(photo.Sizes)
			return &tg.InputPhotoFileLocation{ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference, ThumbSize: kind}, nil
		}
	case *tg.MessageMediaDocument:
		if doc, ok := m.Document.(*tg.Document); ok {
			return &tg.InputDocumentFileLocation{ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference}, nil
		}
	}

	return nil, errNoFile
}

// fileDownload bundles what download needs about the video, file or
// voice note it is about to fetch, keeping download itself within the
// project's own argument limit: where Telegram keeps it, where to
// write it, whether this is an automatic fetch (see connector.MaxBytes)
// and its sender-declared size, 0 when the message carried none.
type fileDownload struct {
	location     tg.InputFileLocationClass
	path         string
	auto         bool
	declaredSize int64
}

// download writes fd's file from Telegram to fd.path, readable only by
// the user, refusing outright a declared size already over the limit
// fd.auto selects (see connector.MaxBytes), and otherwise capping the
// bytes actually received at that declared size plus a small tolerance
// (see connector.ByteCap): the same discipline downloadImage already
// gives a photo, sticker or GIF, just applied to a format Qt's own
// image decoders never touch, so it streams straight to disk rather
// than buffering in memory first.
func download(ctx context.Context, api *tg.Client, fd fileDownload) error {
	max := connector.MaxBytes(fd.auto)
	if fd.declaredSize > max {
		return errMediaTooLarge
	}

	downloadCtx, onWrite, stop := downloadDeadline(ctx, fd.auto)
	defer stop()

	f, err := os.OpenFile(fd.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("telegram: fetch media: %w", err)
	}

	w := &connector.BoundedWriter{W: f, Max: connector.ByteCap(fd.declaredSize, max), OnWrite: onWrite}
	if _, err := downloader.NewDownloader().Download(api, fd.location).Stream(downloadCtx, w); err != nil {
		return failedDownload(fd.path, f, err)
	}

	return f.Close()
}

// failedDownload closes f and removes the partial bytes a failed
// stream already wrote to path, so a refused or aborted download never
// leaves a file behind even for a caller that talks to this connector
// directly rather than through the cache's own cleanup (see
// cache.Cache.Fetch, which would otherwise be the only thing removing
// it). The remove is best effort: if it fails there is nothing more
// useful to do than return the download's own error, which is what the
// caller already needs to see.
func failedDownload(path string, f *os.File, streamErr error) error {
	closeErr := f.Close()
	_ = os.Remove(path) // best effort cleanup; streamErr is the error that matters here

	if errors.Is(streamErr, connector.ErrMediaTooLarge) {
		return errors.Join(errMediaTooLarge, closeErr)
	}

	return errors.Join(fmt.Errorf("telegram: fetch media: %w", streamErr), closeErr)
}

// downloadDeadline bounds one download attempt: the same flat
// fetchTimeout every small, automatic fetch already used, since
// AutoFetchMaxBytes keeps those quick regardless, or an idle timeout
// with a longer absolute ceiling for a user-requested file large
// enough to need one - a slow but steadily arriving transfer should
// not be cut off just because it takes longer than a small file would,
// while a connection that has actually stalled is still caught
// quickly (see connector.IdleContext).
func downloadDeadline(ctx context.Context, auto bool) (downloadCtx context.Context, onWrite func(), stop func()) {
	if auto {
		downloadCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		return downloadCtx, func() {}, cancel
	}

	return connector.IdleContext(ctx, connector.IdleTimeout, connector.AbsoluteCeiling)
}
