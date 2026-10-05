// Package app coordinates normalized conversations, storage, connectors, and
// user-facing events. It contains no service protocol code.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/notify"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

var (
	ErrBadRequest    = errors.New("bad request")
	ErrUnknownMethod = errors.New("unknown method")
	errNoManager     = errors.New("connector manager is unavailable")
)

type settings struct {
	notifications       bool
	notificationPreview bool
	demoChatter         bool
}

// DemoInjector is optionally supplied when a demo connector is configured.
type DemoInjector interface {
	Inject(conversationRemoteID string) (domain.Message, error)
}

// App is the application service and connector Sink. Its public methods are
// the typed operations exposed by the local RPC protocol.
type App struct {
	Store      *store.Store
	Manager    *connector.Manager
	Notifier   notify.Notifier
	Clock      connector.Clock
	Emit       func(name string, data any)
	Demo       bool
	DemoInject DemoInjector
	SetChatter func(enabled bool)

	mu                  sync.RWMutex
	emitMu              sync.Mutex
	settings            settings
	focusedConversation string
	windowActive        bool
}

// New constructs an App with notifications, previews, and demo chatter
// enabled by default. Demo chatter has an effect only in demo mode.
func New(s *store.Store, manager *connector.Manager, notifier notify.Notifier, clock connector.Clock) *App {
	if clock == nil {
		clock = connector.RealClock{}
	}
	return &App{
		Store: s, Manager: manager, Notifier: notifier, Clock: clock,
		settings: settings{notifications: true, notificationPreview: true, demoChatter: true},
	}
}

func (a *App) emit(name string, data any) {
	if a.Emit == nil {
		return
	}
	a.emitMu.Lock()
	a.Emit(name, data)
	a.emitMu.Unlock()
}

func (a *App) unreadTotal() int {
	total, err := a.Store.UnreadTotal()
	if err != nil {
		return 0
	}
	return total
}

func (a *App) emitUnreadIfChanged(before int) {
	if after := a.unreadTotal(); after != before {
		a.emit("unread.changed", struct {
			Total int `json:"total"`
		}{Total: after})
	}
}

func (a *App) emitConversation(id string) (domain.Conversation, error) {
	c, err := a.Store.Conversation(id)
	if err != nil {
		return c, err
	}
	a.emit("conversation.updated", c)
	return c, nil
}

// HelloResult describes the protocol and current application mode.
type HelloResult struct {
	Protocol    int    `json:"protocol"`
	Version     string `json:"version"`
	Demo        bool   `json:"demo"`
	UnreadTotal int    `json:"unreadTotal"`
}

func (a *App) Hello(context.Context, struct{}) (HelloResult, error) {
	return HelloResult{Protocol: 1, Version: "0.2.0", Demo: a.Demo, UnreadTotal: a.unreadTotal()}, nil
}

func (a *App) AccountsList(context.Context, struct{}) ([]domain.Account, error) {
	return a.Store.Accounts()
}

type ConversationsListParams struct {
	Query string `json:"query"`
}

func (a *App) ConversationsList(_ context.Context, params ConversationsListParams) ([]domain.Conversation, error) {
	return a.Store.Conversations(params.Query)
}

type MessagesListParams struct {
	ConversationID string `json:"conversationId"`
	Before         string `json:"before"`
	Limit          int    `json:"limit"`
}

type MessagesListResult struct {
	Messages []domain.Message `json:"messages"`
	HasMore  bool             `json:"hasMore"`
}

func (a *App) MessagesList(_ context.Context, params MessagesListParams) (MessagesListResult, error) {
	if strings.TrimSpace(params.ConversationID) == "" {
		return MessagesListResult{}, fmt.Errorf("%w: conversationId is required", ErrBadRequest)
	}
	limit := params.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return MessagesListResult{}, fmt.Errorf("%w: message limit must be between 1 and 200", ErrBadRequest)
	}
	if _, err := a.Store.Conversation(params.ConversationID); err != nil {
		return MessagesListResult{}, err
	}
	messages, hasMore, err := a.Store.Messages(params.ConversationID, params.Before, limit)
	if err != nil {
		return MessagesListResult{}, err
	}
	return MessagesListResult{Messages: messages, HasMore: hasMore}, nil
}

type SendMessageParams struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
}

func (a *App) SendMessage(ctx context.Context, params SendMessageParams) (domain.Message, error) {
	if strings.TrimSpace(params.ConversationID) == "" {
		return domain.Message{}, fmt.Errorf("%w: conversationId is required", ErrBadRequest)
	}
	text, err := domain.NormalizeOutgoingText(params.Text)
	if err != nil {
		return domain.Message{}, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	conv, err := a.Store.Conversation(params.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}
	before := a.unreadTotal()
	message, inserted, err := a.Store.AddMessage(domain.Message{
		ConversationID: conv.ID, SenderName: "You", Text: text, Outgoing: true,
		Status: domain.StatusPending, Created: a.Clock.Now().UnixMilli(),
	})
	if err != nil {
		return domain.Message{}, err
	}
	if inserted {
		a.emit("message.added", message)
		_, _ = a.emitConversation(conv.ID)
		a.emitUnreadIfChanged(before)
	}
	if a.Manager == nil {
		return a.failOutgoing(message, errNoManager)
	}
	if err := a.Manager.Send(ctx, conv, message); err != nil {
		return a.failOutgoing(message, err)
	}
	return message, nil
}

func (a *App) failOutgoing(message domain.Message, cause error) (domain.Message, error) {
	updated, changed, err := a.Store.UpdateMessageStatus(message.ID, domain.StatusFailed)
	if err != nil {
		return message, errors.Join(cause, err)
	}
	if changed {
		a.emit("message.updated", updated)
	}
	return updated, cause
}

type RetryParams struct {
	MessageID string `json:"messageId"`
}

func (a *App) Retry(ctx context.Context, params RetryParams) (domain.Message, error) {
	message, err := a.Store.Message(params.MessageID)
	if err != nil {
		return message, err
	}
	if !message.Outgoing || message.Status != domain.StatusFailed {
		return message, fmt.Errorf("%w: only failed outgoing messages can be retried", ErrBadRequest)
	}
	conv, err := a.Store.Conversation(message.ConversationID)
	if err != nil {
		return message, err
	}
	updated, changed, err := a.Store.UpdateMessageStatus(message.ID, domain.StatusPending)
	if err != nil {
		return message, err
	}
	if changed {
		a.emit("message.updated", updated)
	}
	if a.Manager == nil {
		return a.failOutgoing(updated, errNoManager)
	}
	if err := a.Manager.Send(ctx, conv, updated); err != nil {
		return a.failOutgoing(updated, err)
	}
	return updated, nil
}

type ConversationParams struct {
	ConversationID string `json:"conversationId"`
}

func (a *App) MarkRead(ctx context.Context, params ConversationParams) (struct{}, error) {
	before := a.unreadTotal()
	changed, err := a.Store.MarkRead(params.ConversationID)
	if err != nil {
		return struct{}{}, err
	}
	if changed {
		_, _ = a.emitConversation(params.ConversationID)
		a.emitUnreadIfChanged(before)
	}
	if a.Manager != nil {
		if conv, err := a.Store.Conversation(params.ConversationID); err == nil {
			if err := a.Manager.MarkRead(ctx, conv); err != nil {
				return struct{}{}, err
			}
		}
	}
	return struct{}{}, nil
}

type SetMutedParams struct {
	ConversationID string `json:"conversationId"`
	Muted          bool   `json:"muted"`
}

func (a *App) SetMuted(_ context.Context, params SetMutedParams) (domain.Conversation, error) {
	before := a.unreadTotal()
	if err := a.Store.SetMuted(params.ConversationID, params.Muted); err != nil {
		return domain.Conversation{}, err
	}
	conv, err := a.Store.Conversation(params.ConversationID)
	if err == nil {
		a.emit("conversation.updated", conv)
		a.emitUnreadIfChanged(before)
	}
	return conv, err
}

type OpenConversationParams struct {
	AccountID string `json:"accountId"`
	ContactID string `json:"contactId"`
}

func (a *App) OpenConversation(_ context.Context, params OpenConversationParams) (domain.Conversation, error) {
	if params.AccountID == "" || params.ContactID == "" {
		return domain.Conversation{}, fmt.Errorf("%w: accountId and contactId are required", ErrBadRequest)
	}
	contact, err := a.Store.Contact(params.AccountID, params.ContactID)
	if err != nil {
		return domain.Conversation{}, err
	}
	conv, created, err := a.Store.EnsureConversation(domain.Conversation{
		AccountID: params.AccountID, RemoteID: contact.RemoteID, Kind: domain.KindDirect, Title: contact.Name,
	})
	if err != nil {
		return conv, err
	}
	if created {
		a.emit("conversation.updated", conv)
	}
	return conv, nil
}

type ContactsListParams struct {
	AccountID string `json:"accountId"`
	Query     string `json:"query"`
}

func (a *App) ContactsList(_ context.Context, params ContactsListParams) ([]domain.Contact, error) {
	if params.AccountID == "" {
		return nil, fmt.Errorf("%w: accountId is required", ErrBadRequest)
	}
	if _, err := a.Store.Account(params.AccountID); err != nil {
		return nil, err
	}
	return a.Store.Contacts(params.AccountID, params.Query)
}

type FocusParams struct {
	ConversationID string `json:"conversationId"`
	WindowActive   bool   `json:"windowActive"`
}

func (a *App) SetFocus(_ context.Context, params FocusParams) (struct{}, error) {
	if params.ConversationID != "" {
		if _, err := a.Store.Conversation(params.ConversationID); err != nil {
			return struct{}{}, err
		}
	}
	a.mu.Lock()
	a.focusedConversation = params.ConversationID
	a.windowActive = params.WindowActive
	a.mu.Unlock()
	return struct{}{}, nil
}

type SettingsParams struct {
	Notifications       bool `json:"notifications"`
	NotificationPreview bool `json:"notificationPreview"`
	DemoChatter         bool `json:"demoChatter"`
}

func (a *App) ApplySettings(_ context.Context, params SettingsParams) (struct{}, error) {
	a.mu.Lock()
	a.settings = settings{
		notifications:       params.Notifications,
		notificationPreview: params.NotificationPreview,
		demoChatter:         params.DemoChatter,
	}
	setChatter := a.SetChatter
	demo := a.Demo
	a.mu.Unlock()
	if demo && setChatter != nil {
		setChatter(params.DemoChatter)
	}
	return struct{}{}, nil
}

type InjectParams struct {
	ConversationID string `json:"conversationId"`
}

func (a *App) Inject(_ context.Context, params InjectParams) (domain.Message, error) {
	if !a.Demo || a.DemoInject == nil {
		return domain.Message{}, ErrUnknownMethod
	}
	conversation, err := a.Store.Conversation(params.ConversationID)
	if err != nil {
		return domain.Message{}, err
	}
	message, err := a.DemoInject.Inject(conversation.RemoteID)
	if err != nil {
		return message, err
	}
	if stored, err := a.Store.MessageByRemote(conversation.ID, message.RemoteID); err == nil {
		return stored, nil
	}
	return message, nil
}

// AccountStatus persists and publishes an account's connection state.
func (a *App) AccountStatus(accountID, status, detail string) {
	account, err := a.Store.SetAccountStatus(accountID, status, detail)
	if err == nil {
		a.emit("account.updated", account)
	}
}

// Contact persists a normalized provider contact.
func (a *App) Contact(contact domain.Contact) {
	_ = a.Store.UpsertContact(contact)
}

// Conversation upserts a normalized provider conversation and publishes it.
func (a *App) Conversation(conversation domain.Conversation) {
	updated, _, err := a.Store.EnsureConversation(conversation)
	if err == nil {
		a.emit("conversation.updated", updated)
	}
}

// Incoming persists a newly received message and applies notification/focus
// policy. Duplicate remote messages do not emit events or notifications.
func (a *App) Incoming(accountID, conversationRemoteID string, message domain.Message) {
	conversation, err := a.Store.ConversationByRemote(accountID, conversationRemoteID)
	if err != nil {
		return
	}
	message.ConversationID = conversation.ID
	if message.Created == 0 {
		message.Created = a.Clock.Now().UnixMilli()
	}
	before := a.unreadTotal()
	message, inserted, err := a.Store.AddMessage(message)
	if err != nil || !inserted {
		return
	}

	a.mu.RLock()
	shouldMarkRead := a.windowActive && a.focusedConversation == conversation.ID
	shouldNotify := a.settings.notifications && !conversation.Muted && !shouldMarkRead
	preview := a.settings.notificationPreview
	a.mu.RUnlock()
	if shouldMarkRead {
		_, _ = a.Store.MarkRead(conversation.ID)
	}
	updated, err := a.Store.Conversation(conversation.ID)
	if err != nil {
		return
	}
	a.emit("message.added", message)
	a.emit("conversation.updated", updated)
	a.emitUnreadIfChanged(before)
	if shouldNotify && a.Notifier != nil {
		title := message.SenderName
		if conversation.Kind == domain.KindGroup {
			title += " · " + conversation.Title
		}
		body := "New message"
		if preview {
			body = message.Text
		}
		a.Notifier.Notify(title, body)
	}
}

// History persists an initial or backfilled message without applying live
// notification or focused-conversation policy.
func (a *App) History(accountID, conversationRemoteID string, message domain.Message) {
	conversation, err := a.Store.ConversationByRemote(accountID, conversationRemoteID)
	if err != nil {
		return
	}
	message.ConversationID = conversation.ID
	if message.Created == 0 {
		message.Created = a.Clock.Now().UnixMilli()
	}
	before := a.unreadTotal()
	message, inserted, err := a.Store.AddMessage(message)
	if err != nil || !inserted {
		return
	}
	a.emit("message.added", message)
	_, _ = a.emitConversation(conversation.ID)
	a.emitUnreadIfChanged(before)
}

// OutgoingStatus publishes a valid delivery-state change.
func (a *App) OutgoingStatus(localMessageID, remoteID, status string) {
	if remoteID != "" {
		_ = a.Store.SetMessageRemoteID(localMessageID, remoteID)
	}
	message, changed, err := a.Store.UpdateMessageStatus(localMessageID, status)
	if err == nil && changed {
		a.emit("message.updated", message)
	}
}

// Typing publishes a provider typing indicator.
func (a *App) Typing(accountID, conversationRemoteID, name string, active bool) {
	conversation, err := a.Store.ConversationByRemote(accountID, conversationRemoteID)
	if err != nil {
		return
	}
	a.emit("typing", struct {
		ConversationID string `json:"conversationId"`
		Name           string `json:"name"`
		Active         bool   `json:"active"`
	}{ConversationID: conversation.ID, Name: name, Active: active})
}

var _ connector.Sink = (*App)(nil)
var _ connector.HistorySink = (*App)(nil)
