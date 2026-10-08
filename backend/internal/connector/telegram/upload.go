package telegram

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// sendRequest sends m's text, or uploads its attachment and sends that
// with a caption, to peer, threading it under the message m.ReplyTo
// names, if any.
func sendRequest(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, m domain.Message) (tg.UpdatesClass, error) {
	replyTo := inputReplyTo(m.ReplyTo)
	entities := outgoingEntities(m.Mentions)
	if m.Media == nil || m.Media.Kind == domain.MediaLink {
		return api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer: peer, Message: m.Text, RandomID: randomID(), ReplyTo: replyTo, Entities: entities,
		})
	}

	media, err := uploadMedia(ctx, api, m.Media)
	if err != nil {
		return nil, err
	}

	return api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer: peer, Media: media, Message: wireCaption(m), RandomID: randomID(), ReplyTo: replyTo, Entities: entities,
	})
}

// wireCaption is a message's text to send as its media's caption: empty
// when it is only the placeholder messageText gives media sent with no
// caption, which must never reach the other side as real text.
func wireCaption(m domain.Message) string {
	if m.Media != nil && m.Text == domain.MediaPlaceholder(m.Media.Kind) {
		return ""
	}

	return m.Text
}

// uploadMedia uploads an attachment's local file and describes it as
// Telegram media to send: a photo for a photo, a document with its name
// and MIME type otherwise.
func uploadMedia(ctx context.Context, api *tg.Client, media *domain.Media) (tg.InputMediaClass, error) {
	file, err := uploader.NewUploader(api).FromPath(ctx, media.Path)
	if err != nil {
		return nil, fmt.Errorf("telegram: upload: %w", err)
	}

	if media.Kind == domain.MediaPhoto {
		return &tg.InputMediaUploadedPhoto{File: file}, nil
	}

	attrs := []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: media.FileName}}

	return &tg.InputMediaUploadedDocument{File: file, MimeType: sniffMIME(media.Path), Attributes: attrs}, nil
}

// sniffMIME reads a file's first bytes to say its content type, or a
// generic fallback when it cannot be read.
func sniffMIME(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close() // reading only; a close failure changes nothing we already read

	buf := make([]byte, 512)
	n, _ := f.Read(buf) // a short or empty read still sniffs from what came back

	return http.DetectContentType(buf[:n])
}
