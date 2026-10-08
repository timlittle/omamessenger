package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

// MaxPageSize is the most messages one Messages call returns.
const MaxPageSize = 200

// mediaLabels are the placeholder texts a connector gives a message whose
// media it could not describe when it was first synced.
var mediaLabels = []string{"[Photo]", "[Video]", "[File]", "[Voice message]"}

// Messages returns up to limit messages before beforeID, oldest first,
// whether older ones remain, and whether older history could not be
// fetched from the service right now: the page already loaded still
// stands either way (see olderFromService), and the UI shows this as a
// small note rather than silently stopping. A zero limit means the
// default page size.
func (c *Commands) Messages(ctx context.Context, conversationID, beforeID string, limit int) (messages []domain.Message, hasMore, historyUnavailable bool, err error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, false, false, fmt.Errorf("%w: conversationId is required", ErrInvalidInput)
	}

	if limit < 0 || limit > MaxPageSize {
		return nil, false, false, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidInput, MaxPageSize)
	}

	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return nil, false, false, err
	}

	page, more, err := c.store.Messages(ctx, conversationID, beforeID, limit)
	unavailable := false
	if err == nil && !more && c.history != nil {
		page, more, unavailable, err = c.olderFromService(ctx, conv, beforeID, limit, page)
	}
	if err != nil {
		return page, more, unavailable, err
	}

	return c.refreshStaleMedia(ctx, conv, beforeID, limit, page), more, unavailable, nil
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
// loaded stands: scrolling back is not worth an error. A failure
// wrapping connector.ErrHistoryUnavailable, such as WhatsApp's phone
// never answering an on-demand request, is reported back as the third
// return value instead of silently swallowed like any other failure, so
// Messages can tell the UI the difference between "try again later" and
// "there is nothing more".
func (c *Commands) olderFromService(ctx context.Context, conv domain.Conversation, beforeID string, limit int, page []domain.Message) ([]domain.Message, bool, bool, error) {
	oldest, err := c.store.OldestRemoteID(ctx, conv.ID)
	if err != nil {
		return page, false, false, nil
	}

	loaded, err := c.history.LoadOlder(ctx, conv, oldest, max(limit, store.DefaultPageSize))
	if err != nil {
		return page, false, errors.Is(err, connector.ErrHistoryUnavailable), nil
	}
	if loaded == 0 {
		return page, false, false, nil
	}

	page, _, err = c.store.Messages(ctx, conv.ID, beforeID, limit)

	// The service may hold more still; the next page asks it again.
	return page, true, false, err
}

// SendOptions are a message's optional extras, bundled into one
// argument to keep Send's own argument count within this codebase's
// limit: an attachment to send with text as its caption, the local id
// of a message in the same conversation this one answers, and the
// "@name" tokens the composer inserted into text. Each is "" or nil
// when the message has none.
type SendOptions struct {
	AttachmentPath string
	ReplyToID      string
	Mentions       []domain.Mention
}

// Send stores a message as pending, publishes it and hands it to the
// service. If the service refuses it, the message is returned as
// failed, ready to retry; that is not an error.
func (c *Commands) Send(ctx context.Context, conversationID, text string, opts SendOptions) (domain.Message, error) {
	id := ""
	if opts.AttachmentPath != "" {
		id = newAttachmentID()
	}

	media, err := c.prepareAttachment(ctx, id, opts.AttachmentPath)
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

	replyTo, err := c.resolveReplyTo(ctx, conv.ID, opts.ReplyToID)
	if err != nil {
		return domain.Message{}, err
	}

	before := c.events.unreadTotal(ctx)
	m, _, err := c.store.AddMessage(ctx, domain.Message{
		ID: id, ConversationID: conv.ID, SenderName: "You", Text: text, Outgoing: true,
		Status: domain.StatusPending, Created: time.Now().UnixMilli(), Media: media, ReplyTo: replyTo, Mentions: opts.Mentions,
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

	if err := c.checkAttachmentAvailable(m); err != nil {
		return m, err
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

// checkAttachmentAvailable reports a clear, safe error when m's
// attachment has been swept from the outgoing area (see cache.Outgoing's
// retention and size limit) before it could be retried.
func (c *Commands) checkAttachmentAvailable(m domain.Message) error {
	if m.Media == nil || c.outgoing == nil {
		return nil
	}

	path := c.outgoing.Path(m.ID, m.Media.FileName)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%w: the attachment is no longer available; attach it again", ErrInvalidInput)
	}

	return nil
}

// dispatch hands a pending message to the service, marking it failed if
// the service refuses it. A fast connector can confirm delivery, and
// update the stored message, before Send returns; the reply must carry
// that update rather than the message as it was handed over, because the
// UI applies whatever this returns to its timeline, and it would
// otherwise arrive after the sent notification and regress the bubble
// back to pending with nothing left to correct it.
func (c *Commands) dispatch(ctx context.Context, conv domain.Conversation, m domain.Message) (domain.Message, error) {
	release := c.reserveAttachment(m)
	defer release()

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

// reserveAttachment marks m's attachment, if it has one, as being sent
// right now, so the outgoing area's background sweep never removes it
// mid-upload; the returned func releases the mark once the send ends.
func (c *Commands) reserveAttachment(m domain.Message) func() {
	if m.Media == nil || c.outgoing == nil {
		return func() {}
	}

	return c.outgoing.Reserve(m.ID)
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
