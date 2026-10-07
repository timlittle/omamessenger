package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// MaxPageSize is the most messages one Messages call returns.
const MaxPageSize = 200

// mediaLabels are the placeholder texts a connector gives a message whose
// media it could not describe when it was first synced.
var mediaLabels = []string{"[Photo]", "[Video]", "[File]", "[Voice message]"}

// Messages returns up to limit messages before beforeID, oldest first, and
// whether older ones remain. A zero limit means the default page size.
func (c *Commands) Messages(ctx context.Context, conversationID, beforeID string, limit int) ([]domain.Message, bool, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, false, fmt.Errorf("%w: conversationId is required", ErrInvalidInput)
	}

	if limit < 0 || limit > MaxPageSize {
		return nil, false, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidInput, MaxPageSize)
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return nil, false, err
	}

	page, more, err := c.store.Messages(ctx, conversationID, beforeID, limit)
	if err == nil && !more && c.history != nil {
		page, more, err = c.olderFromService(ctx, conv, beforeID, limit, page)
	}
	if err != nil {
		return page, more, err
	}

	return c.refreshStaleMedia(ctx, conv, beforeID, limit, page), more, nil
}

// refreshStaleMedia asks the service to re-report any messages in page
// that were stored before it reported their media or a link preview, then
// rereads the page so the caller sees what came back. Each message id is
// asked for once per helper run; if the service fails, the stored page
// stands, the same as a failed older-history fetch.
func (c *Commands) refreshStaleMedia(ctx context.Context, conv domain.Conversation, beforeID string, limit int, page []domain.Message) []domain.Message {
	if c.refresher == nil {
		return page
	}

	var stale []string
	for _, m := range page {
		if needsRefresh(m) {
			stale = append(stale, m.RemoteID)
		}
	}

	fresh := c.refreshed.take(stale)
	if len(fresh) == 0 {
		return page
	}

	if err := c.refresher.RefreshMessages(ctx, conv, fresh); err != nil {
		return page
	}

	reloaded, _, err := c.store.Messages(ctx, conv.ID, beforeID, limit)
	if err != nil {
		return page
	}

	return reloaded
}

// needsRefresh reports whether a stored message is missing media it
// likely has: a connector's placeholder text for media, or a link worth
// a preview.
func needsRefresh(m domain.Message) bool {
	if m.Media != nil || m.RemoteID == "" {
		return false
	}

	return slices.Contains(mediaLabels, m.Text) || strings.Contains(m.Text, "://")
}

// olderFromService fetches history the store does not have yet from the
// conversation's service, then pages again. Older history is not new, so
// the conversation keeps the unread count it had: it stores through
// Ingest.History, which never counts a message towards unread, so there
// is nothing here to restore or correct, and nothing for a
// conversations.markRead running concurrently for the same chat (the UI
// sends both when a chat opens) to race against. Ingest.History already
// announces the conversation's current preview and activity as each
// message lands. If the service cannot be reached, the page already
// loaded stands: scrolling back is not worth an error.
func (c *Commands) olderFromService(ctx context.Context, conv domain.Conversation, beforeID string, limit int, page []domain.Message) ([]domain.Message, bool, error) {
	oldest, err := c.store.OldestRemoteID(ctx, conv.ID)
	if err != nil {
		return page, false, nil
	}

	loaded, err := c.history.LoadOlder(ctx, conv, oldest, max(limit, store.DefaultPageSize))
	if err != nil || loaded == 0 {
		return page, false, nil
	}

	page, _, err = c.store.Messages(ctx, conv.ID, beforeID, limit)

	// The service may hold more still; the next page asks it again.
	return page, true, err
}

// Send stores a message as pending, publishes it and hands it to the
// service. attachmentPath names a file on this machine to send along
// with text as its caption, or "" for a plain text message. replyToID,
// when not "", is the local id of a message in the same conversation
// this one answers. If the service refuses it, the message is returned
// as failed, ready to retry; that is not an error.
func (c *Commands) Send(ctx context.Context, conversationID, text, attachmentPath, replyToID string) (domain.Message, error) {
	id := ""
	if attachmentPath != "" {
		id = newAttachmentID()
	}

	media, err := c.prepareAttachment(ctx, id, attachmentPath)
	if err != nil {
		return domain.Message{}, err
	}

	text, err = captionText(text, media)
	if err != nil {
		return domain.Message{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return domain.Message{}, err
	}

	replyTo, err := c.resolveReplyTo(ctx, conv.ID, replyToID)
	if err != nil {
		return domain.Message{}, err
	}

	before := c.events.unreadTotal(ctx)
	m, _, err := c.store.AddMessage(ctx, domain.Message{
		ID: id, ConversationID: conv.ID, SenderName: "You", Text: text, Outgoing: true,
		Status: domain.StatusPending, Created: time.Now().UnixMilli(), Media: media, ReplyTo: replyTo,
	})
	if err != nil {
		return domain.Message{}, err
	}

	c.events.publish(ctx, EventMessageAdded, m)
	c.events.conversationChanged(ctx, conv.ID, before)

	return c.dispatch(ctx, conv, m)
}

// resolveReplyTo turns a local message id to reply to into the Reply a
// connector can thread the new message under: the quoted message's
// remote id (empty if the service has not assigned one yet), its sender
// and a short excerpt of its text. An empty replyToID means no reply.
func (c *Commands) resolveReplyTo(ctx context.Context, conversationID, replyToID string) (*domain.Reply, error) {
	if replyToID == "" {
		return nil, nil
	}

	quoted, err := c.store.Message(ctx, replyToID)
	if err != nil {
		return nil, err
	}

	if quoted.ConversationID != conversationID {
		return nil, fmt.Errorf("%w: replyTo must be a message in the conversation", ErrInvalidInput)
	}

	return &domain.Reply{RemoteID: quoted.RemoteID, SenderName: quoted.SenderName, Text: domain.Excerpt(quoted.Text)}, nil
}

// captionText validates a message's text, required unless media stands
// in for it: a photo, video or file with no caption still gets the
// placeholder every stored message needs, which the UI shows as no
// caption at all (see domain.MediaPlaceholder).
func captionText(text string, media *domain.Media) (string, error) {
	text = strings.TrimSpace(text)
	if text != "" {
		if utf8.RuneCountInString(text) > domain.MaxTextLength {
			return "", domain.ErrTextTooLong
		}

		return text, nil
	}

	if media == nil {
		return "", domain.ErrEmptyText
	}

	return domain.MediaPlaceholder(media.Kind), nil
}

// Retry sends a failed outgoing message again, re-reading an attachment's
// stored copy since a stored message does not keep its path.
func (c *Commands) Retry(ctx context.Context, messageID string) (domain.Message, error) {
	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return m, err
	}

	if !m.Outgoing || m.Status != domain.StatusFailed {
		return m, fmt.Errorf("%w: only failed outgoing messages can be retried", ErrInvalidInput)
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return m, err
	}

	m, err = c.setStatus(ctx, m, domain.StatusPending)
	if err != nil {
		return m, err
	}

	// setStatus reloads the message from the store, which never keeps an
	// attachment's local path (see domain.Media.Path), so it is restored
	// here, right before the connector needs it.
	if m.Media != nil && c.outgoing != nil {
		m.Media.Path = c.outgoing.Path(m.ID, m.Media.FileName)
	}

	return c.dispatch(ctx, conv, m)
}

// dispatch hands a pending message to the service, marking it failed if
// the service refuses it. A fast connector can confirm delivery, and
// update the stored message, before Send returns; the reply must carry
// that update rather than the message as it was handed over, because the
// UI applies whatever this returns to its timeline, and it would
// otherwise arrive after the sent notification and regress the bubble
// back to pending with nothing left to correct it.
func (c *Commands) dispatch(ctx context.Context, conv domain.Conversation, m domain.Message) (domain.Message, error) {
	if err := c.dispatcher.Send(ctx, conv, m); err != nil {
		return c.setStatus(ctx, m, domain.StatusFailed)
	}

	current, err := c.store.Message(ctx, m.ID)
	if err != nil {
		return m, nil
	}

	// The store never keeps an attachment's local path (see
	// domain.Media.Path), so it is carried over from the message handed
	// to the dispatcher, the same as Retry restores it.
	if m.Media != nil && current.Media != nil {
		current.Media.Path = m.Media.Path
	}

	return current, nil
}

// setStatus records a delivery status and publishes the change.
func (c *Commands) setStatus(ctx context.Context, m domain.Message, status string) (domain.Message, error) {
	updated, changed, err := c.store.UpdateMessageStatus(ctx, m.ID, status)
	if err != nil {
		return m, err
	}

	if changed {
		c.events.publish(ctx, EventMessageUpdated, updated)
	}

	return updated, nil
}
