package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errNoFile reports a message with no photo or file to download.
var errNoFile = errors.New("telegram: the message has nothing to download")

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

	return download(ctx, api, location, path)
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
			size, _, _ := largest(photo.Sizes)
			return &tg.InputPhotoFileLocation{ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference, ThumbSize: size}, nil
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
