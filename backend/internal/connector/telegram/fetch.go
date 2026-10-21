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
// permanent failure, never retried.
var errMediaTooLarge = errors.New("telegram: media exceeds the size limit")

// fetchTimeout bounds how long one image download attempt waits for
// Telegram's servers, so a stalled download cannot leave a caller
// waiting forever.
const fetchTimeout = 2 * time.Minute

// FetchMedia downloads the photo or file of the message Telegram numbers
// messageRemoteID to path. A message's file reference expires, so the
// message is read again for a fresh one first.
func (c *Connector) FetchMedia(ctx context.Context, conv domain.Conversation, messageRemoteID, path string) error {
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

	profile, ok := imageProfile(msg.Media)
	if !ok {
		return download(ctx, api, location, path)
	}

	if size, known := declaredMediaSize(msg.Media); known && size > profile.MaxInputLen {
		return errMediaTooLarge
	}

	return downloadImage(ctx, api, location, path, profile)
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

// downloadImage downloads location into memory, bounded by profile's
// own MaxInputLen even if Telegram's declared size undersold what it
// actually streams (see boundedBuffer, the backstop under FetchMedia's
// own declaredMediaSize check), and writes the result to path through
// the sanitizer every photo, sticker or GIF must pass before the UI
// can load it into a QML Image (see connector.SaveImageMedia). A video
// or other document instead streams straight to disk through download
// below: formats Qt's image decoders never touch, so buffering them in
// memory first would cost without ever paying for anything.
func downloadImage(ctx context.Context, api *tg.Client, location tg.InputFileLocationClass, path string, profile imagesafe.Profile) error {
	downloadCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	buf := &boundedBuffer{max: profile.MaxInputLen}
	if _, err := downloader.NewDownloader().Download(api, location).Stream(downloadCtx, buf); err != nil {
		if errors.Is(err, errMediaTooLarge) {
			return errMediaTooLarge
		}

		return fmt.Errorf("telegram: fetch media: %w", err)
	}

	if err := connector.SaveImageMedia(path, buf.Bytes(), profile); err != nil {
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

// download writes a file from Telegram to path, readable only by the user.
func download(ctx context.Context, api *tg.Client, location tg.InputFileLocationClass, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("telegram: fetch media: %w", err)
	}

	if _, err := downloader.NewDownloader().Download(api, location).Stream(ctx, f); err != nil {
		return errors.Join(fmt.Errorf("telegram: fetch media: %w", err), f.Close())
	}

	return f.Close()
}
