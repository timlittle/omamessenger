package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Commands implements the C3 methods the UI calls. Each method takes its
// typed params and returns its typed result; package api handles the wire.
type Commands struct {
	repo       Repository
	pub        *publisher
	session    *session
	clock      connector.Clock
	version    string
	demo       bool
	demoInject DemoInjector
	setChatter func(enabled bool)

	mu         sync.RWMutex
	dispatcher Dispatcher
}

// AttachDispatcher supplies the connector Dispatcher. It is separate from New
// because the Dispatcher (the connector Manager) needs the Ingest half of the
// application as its sink before it can be built.
func (c *Commands) AttachDispatcher(d Dispatcher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dispatcher = d
}

func (c *Commands) currentDispatcher() Dispatcher {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dispatcher
}

// DemoMode reports whether the helper runs on seeded demo data, which
// enables the demo-only protocol methods.
func (c *Commands) DemoMode() bool { return c.demo }

// HelloResult describes the protocol and current application mode.
type HelloResult struct {
	Protocol    int    `json:"protocol"`
	Version     string `json:"version"`
	Demo        bool   `json:"demo"`
	UnreadTotal int    `json:"unreadTotal"`
}

func (c *Commands) Hello(context.Context, struct{}) (HelloResult, error) {
	return HelloResult{Protocol: 1, Version: c.version, Demo: c.demo, UnreadTotal: c.pub.unreadTotal()}, nil
}

func (c *Commands) AccountsList(context.Context, struct{}) ([]domain.Account, error) {
	return c.repo.Accounts()
}

type ConversationsListParams struct {
	Query string `json:"query"`
}

func (c *Commands) ConversationsList(_ context.Context, params ConversationsListParams) ([]domain.Conversation, error) {
	return c.repo.Conversations(params.Query)
}

type ConversationParams struct {
	ConversationID string `json:"conversationId"`
}

// MarkRead clears the unread count locally, then tells the owning connector.
func (c *Commands) MarkRead(ctx context.Context, params ConversationParams) (struct{}, error) {
	before := c.pub.unreadTotal()
	changed, err := c.repo.MarkRead(params.ConversationID)
	if err != nil {
		return struct{}{}, err
	}
	conv, err := c.repo.Conversation(params.ConversationID)
	if err != nil {
		return struct{}{}, err
	}
	if changed {
		c.pub.send("conversation.updated", conv)
		c.pub.unreadChanged(before)
	}
	if d := c.currentDispatcher(); d != nil {
		return struct{}{}, d.MarkRead(ctx, conv)
	}
	return struct{}{}, nil
}

type SetMutedParams struct {
	ConversationID string `json:"conversationId"`
	Muted          bool   `json:"muted"`
}

func (c *Commands) SetMuted(_ context.Context, params SetMutedParams) (domain.Conversation, error) {
	before := c.pub.unreadTotal()
	if err := c.repo.SetMuted(params.ConversationID, params.Muted); err != nil {
		return domain.Conversation{}, err
	}
	conv, err := c.pub.conversation(params.ConversationID)
	if err == nil {
		c.pub.unreadChanged(before)
	}
	return conv, err
}

type OpenConversationParams struct {
	AccountID string `json:"accountId"`
	ContactID string `json:"contactId"`
}

// OpenConversation returns the direct conversation with a contact, creating
// it when it does not exist yet.
func (c *Commands) OpenConversation(_ context.Context, params OpenConversationParams) (domain.Conversation, error) {
	if params.AccountID == "" || params.ContactID == "" {
		return domain.Conversation{}, fmt.Errorf("%w: accountId and contactId are required", ErrBadRequest)
	}
	contact, err := c.repo.Contact(params.AccountID, params.ContactID)
	if err != nil {
		return domain.Conversation{}, err
	}
	conv, created, err := c.repo.EnsureConversation(domain.Conversation{
		AccountID: params.AccountID, RemoteID: contact.RemoteID, Kind: domain.KindDirect, Title: contact.Name,
	})
	if err == nil && created {
		c.pub.send("conversation.updated", conv)
	}
	return conv, err
}

type ContactsListParams struct {
	AccountID string `json:"accountId"`
	Query     string `json:"query"`
}

func (c *Commands) ContactsList(_ context.Context, params ContactsListParams) ([]domain.Contact, error) {
	if params.AccountID == "" {
		return nil, fmt.Errorf("%w: accountId is required", ErrBadRequest)
	}
	if _, err := c.repo.Account(params.AccountID); err != nil {
		return nil, err
	}
	return c.repo.Contacts(params.AccountID, params.Query)
}

type FocusParams struct {
	ConversationID string `json:"conversationId"`
	WindowActive   bool   `json:"windowActive"`
}

// SetFocus records which conversation the user is looking at, which decides
// whether arriving messages are read immediately and whether they notify.
func (c *Commands) SetFocus(_ context.Context, params FocusParams) (struct{}, error) {
	if params.ConversationID != "" {
		if _, err := c.repo.Conversation(params.ConversationID); err != nil {
			return struct{}{}, err
		}
	}
	c.session.setFocus(params.ConversationID, params.WindowActive)
	return struct{}{}, nil
}

type SettingsParams struct {
	Notifications       bool `json:"notifications"`
	NotificationPreview bool `json:"notificationPreview"`
	DemoChatter         bool `json:"demoChatter"`
}

func (c *Commands) ApplySettings(_ context.Context, params SettingsParams) (struct{}, error) {
	c.session.apply(settings{
		notifications:       params.Notifications,
		notificationPreview: params.NotificationPreview,
		demoChatter:         params.DemoChatter,
	})
	if c.demo && c.setChatter != nil {
		c.setChatter(params.DemoChatter)
	}
	return struct{}{}, nil
}

type InjectParams struct {
	ConversationID string `json:"conversationId"`
}

// Inject asks the demo connector to deliver a message into a conversation.
func (c *Commands) Inject(_ context.Context, params InjectParams) (domain.Message, error) {
	if !c.demo || c.demoInject == nil {
		return domain.Message{}, ErrUnknownMethod
	}
	conv, err := c.repo.Conversation(params.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}
	message, err := c.demoInject.Inject(conv.RemoteID)
	if err != nil {
		return message, err
	}
	if stored, err := c.repo.MessageByRemote(conv.ID, message.RemoteID); err == nil {
		return stored, nil
	}
	return message, nil
}
