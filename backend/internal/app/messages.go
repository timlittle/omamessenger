package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

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
// the conversation keeps the unread count it had. If the service cannot
// be reached, the page already loaded stands: scrolling back is not worth
// an error.
func (c *Commands) olderFromService(ctx context.Context, conv domain.Conversation, beforeID string, limit int, page []domain.Message) ([]domain.Message, bool, error) {
	oldest, err := c.store.OldestRemoteID(ctx, conv.ID)
	if err != nil {
		return page, false, nil
	}

	loaded, err := c.history.LoadOlder(ctx, conv, oldest, max(limit, store.DefaultPageSize))
	if err != nil || loaded == 0 {
		return page, false, nil
	}

	// Storing the older messages announced rising totals; announce the
	// corrected one even though it matches the total before the load.
	if changed, _ := c.store.SetUnread(ctx, conv.ID, conv.Unread); changed { // a failure leaves a count the next sync corrects
		c.events.conversationChanged(ctx, conv.ID, -1)
	}

	page, _, err = c.store.Messages(ctx, conv.ID, beforeID, limit)

	// The service may hold more still; the next page asks it again.
	return page, true, err
}

// Send stores a message as pending, publishes it and hands it to the
// service. If the service refuses it, the message is returned as failed,
// ready to retry; that is not an error. replyToID, when not "", is the
// local id of a message in the same conversation this one answers.
func (c *Commands) Send(ctx context.Context, conversationID, text, replyToID string) (domain.Message, error) {
	text, err := domain.NormalizeOutgoingText(text)
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
		ConversationID: conv.ID, SenderName: "You", Text: text, Outgoing: true,
		Status: domain.StatusPending, Created: time.Now().UnixMilli(), ReplyTo: replyTo,
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

// Retry sends a failed outgoing message again.
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

	return c.dispatch(ctx, conv, m)
}

// dispatch hands a pending message to the service, marking it failed if
// the service refuses it.
func (c *Commands) dispatch(ctx context.Context, conv domain.Conversation, m domain.Message) (domain.Message, error) {
	if err := c.dispatcher.Send(ctx, conv, m); err != nil {
		return c.setStatus(ctx, m, domain.StatusFailed)
	}

	return m, nil
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
