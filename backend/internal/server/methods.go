package server

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// method decodes a request's params and runs it.
type method func(ctx context.Context, params json.RawMessage) (any, error)

// bind adapts a handler taking typed params to a method.
func bind[P any](handle func(ctx context.Context, p P) (any, error)) method {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p P
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("%w: %w", errBadParams, err)
		}

		return handle(ctx, p)
	}
}

// methods is the protocol's method table. fake.inject exists only in test
// builds with fake connectors, so a real helper answers it as an unknown
// method.
func methods(c *app.Commands, version string) map[string]method {
	table := map[string]method{
		"hello":                  bind(hello(c, version)),
		"accounts.list":          bind(accountsList(c)),
		"accounts.add":           bind(accountsAdd(c)),
		"accounts.remove":        bind(accountsRemove(c)),
		"auth.submit":            bind(authSubmit(c)),
		"contacts.list":          bind(contactsList(c)),
		"conversations.list":     bind(conversationsList(c)),
		"conversations.open":     bind(conversationsOpen(c)),
		"conversations.markRead": bind(conversationsMarkRead(c)),
		"conversations.setMuted": bind(conversationsSetMuted(c)),
		"messages.list":          bind(messagesList(c)),
		"messages.send":          bind(messagesSend(c)),
		"messages.retry":         bind(messagesRetry(c)),
		"media.fetch":            bind(mediaFetch(c)),
		"ui.setFocus":            bind(uiSetFocus(c)),
		"settings.apply":         bind(settingsApply(c)),
	}

	if c.Faked() {
		table["fake.inject"] = bind(fakeInject(c))
	}

	return table
}

// none is the params of a method that takes none, and the result of one
// that returns nothing; it encodes as {}.
type none struct{}

// helloResult describes the helper to the UI when it connects. Services
// is the messaging services available to add an account for, omitted
// when the helper cannot list them.
type helloResult struct {
	Protocol    int              `json:"protocol"`
	Version     string           `json:"version"`
	UnreadTotal int              `json:"unreadTotal"`
	Services    []domain.Service `json:"services,omitempty"`
}

// hello reports the protocol version, the helper version, the unread
// total and the services available to add an account for.
func hello(c *app.Commands, version string) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		return helloResult{Protocol: Protocol, Version: version, UnreadTotal: c.UnreadTotal(ctx), Services: c.Services()}, nil
	}
}

// accountsList lists the signed-in accounts.
func accountsList(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		return c.Accounts(ctx)
	}
}

// addAccountParams names the service to add an account for. apiId and
// apiHash are kept for Telegram accounts given their own API keys from
// my.telegram.org; options is the general form other providers read
// their own setup from. Both ways reach the provider as options.
type addAccountParams struct {
	Service string            `json:"service"`
	APIID   int               `json:"apiId"`
	APIHash string            `json:"apiHash"`
	Options map[string]string `json:"options"`
}

// accountsAdd adds an account and starts signing it in.
func accountsAdd(c *app.Commands) func(context.Context, addAccountParams) (any, error) {
	return func(ctx context.Context, p addAccountParams) (any, error) {
		return c.AddAccount(ctx, app.NewAccount{Service: p.Service, Options: setupOptions(p)})
	}
}

// setupOptions merges a request's options object with its apiId and
// apiHash fields, the long-standing way to give Telegram's own API keys.
func setupOptions(p addAccountParams) map[string]string {
	options := maps.Clone(p.Options)
	if p.APIID == 0 && p.APIHash == "" {
		return options
	}

	if options == nil {
		options = map[string]string{}
	}
	if p.APIID != 0 {
		options["apiId"] = strconv.Itoa(p.APIID)
	}
	if p.APIHash != "" {
		options["apiHash"] = p.APIHash
	}

	return options
}

// accountParams names one account.
type accountParams struct {
	AccountID string `json:"accountId"`
}

// accountsRemove signs an account out and deletes it.
func accountsRemove(c *app.Commands) func(context.Context, accountParams) (any, error) {
	return func(ctx context.Context, p accountParams) (any, error) {
		return none{}, c.RemoveAccount(ctx, p.AccountID)
	}
}

// authParams answers a sign-in step.
type authParams struct {
	AccountID string `json:"accountId"`
	Step      string `json:"step"`
	Value     string `json:"value"`
}

// authSubmit answers the step an account's sign-in asked for.
func authSubmit(c *app.Commands) func(context.Context, authParams) (any, error) {
	return func(ctx context.Context, p authParams) (any, error) {
		return none{}, c.SubmitAuth(ctx, p.AccountID, p.Step, p.Value)
	}
}

// contactsParams selects contacts of one account by name.
type contactsParams struct {
	AccountID string `json:"accountId"`
	Query     string `json:"query"`
}

// contactsList lists an account's contacts matching a query.
func contactsList(c *app.Commands) func(context.Context, contactsParams) (any, error) {
	return func(ctx context.Context, p contactsParams) (any, error) {
		return c.Contacts(ctx, p.AccountID, p.Query)
	}
}

// conversationsParams filters the conversation list.
type conversationsParams struct {
	Query string `json:"query"`
}

// conversationsList lists conversations, newest first.
func conversationsList(c *app.Commands) func(context.Context, conversationsParams) (any, error) {
	return func(ctx context.Context, p conversationsParams) (any, error) {
		return c.Conversations(ctx, p.Query)
	}
}

// openParams names a contact to chat with.
type openParams struct {
	AccountID string `json:"accountId"`
	ContactID string `json:"contactId"`
}

// conversationsOpen returns the direct conversation with a contact.
func conversationsOpen(c *app.Commands) func(context.Context, openParams) (any, error) {
	return func(ctx context.Context, p openParams) (any, error) {
		return c.OpenConversation(ctx, p.AccountID, p.ContactID)
	}
}

// conversationParams names one conversation.
type conversationParams struct {
	ConversationID string `json:"conversationId"`
}

// conversationsMarkRead marks a conversation read.
func conversationsMarkRead(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		return none{}, c.MarkRead(ctx, p.ConversationID)
	}
}

// mutedParams mutes or unmutes a conversation.
type mutedParams struct {
	ConversationID string `json:"conversationId"`
	Muted          bool   `json:"muted"`
}

// conversationsSetMuted mutes or unmutes a conversation.
func conversationsSetMuted(c *app.Commands) func(context.Context, mutedParams) (any, error) {
	return func(ctx context.Context, p mutedParams) (any, error) {
		return c.SetMuted(ctx, p.ConversationID, p.Muted)
	}
}

// messagesParams selects a page of messages.
type messagesParams struct {
	ConversationID string `json:"conversationId"`
	Before         string `json:"before"`
	Limit          int    `json:"limit"`
}

// messagesResult is a page of messages, oldest first.
type messagesResult struct {
	Messages []domain.Message `json:"messages"`
	HasMore  bool             `json:"hasMore"`
}

// messagesList returns a page of a conversation's messages.
func messagesList(c *app.Commands) func(context.Context, messagesParams) (any, error) {
	return func(ctx context.Context, p messagesParams) (any, error) {
		page, more, err := c.Messages(ctx, p.ConversationID, p.Before, p.Limit)
		return messagesResult{Messages: page, HasMore: more}, err
	}
}

// sendParams is a message to send.
type sendParams struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
}

// messagesSend sends a message.
func messagesSend(c *app.Commands) func(context.Context, sendParams) (any, error) {
	return func(ctx context.Context, p sendParams) (any, error) {
		return c.Send(ctx, p.ConversationID, p.Text)
	}
}

// retryParams names one message: a failed one to retry, or one whose
// media to fetch.
type retryParams struct {
	MessageID string `json:"messageId"`
}

// messagesRetry sends a failed message again.
func messagesRetry(c *app.Commands) func(context.Context, retryParams) (any, error) {
	return func(ctx context.Context, p retryParams) (any, error) {
		return c.Retry(ctx, p.MessageID)
	}
}

// mediaResult is where a downloaded photo, video or file is on disk.
type mediaResult struct {
	Path string `json:"path"`
}

// mediaFetch downloads a message's media, or finds it already cached.
func mediaFetch(c *app.Commands) func(context.Context, retryParams) (any, error) {
	return func(ctx context.Context, p retryParams) (any, error) {
		path, err := c.FetchMedia(ctx, p.MessageID)

		return mediaResult{Path: path}, err
	}
}

// focusParams says which conversation the user is looking at.
type focusParams struct {
	ConversationID string `json:"conversationId"`
	WindowActive   bool   `json:"windowActive"`
}

// uiSetFocus records which conversation the user is looking at.
func uiSetFocus(c *app.Commands) func(context.Context, focusParams) (any, error) {
	return func(ctx context.Context, p focusParams) (any, error) {
		return none{}, c.SetFocus(ctx, p.ConversationID, p.WindowActive)
	}
}

// settingsParams are the plugin settings.
type settingsParams struct {
	Notifications       bool `json:"notifications"`
	NotificationPreview bool `json:"notificationPreview"`
}

// settingsApply replaces the user's settings.
func settingsApply(c *app.Commands) func(context.Context, settingsParams) (any, error) {
	return func(_ context.Context, p settingsParams) (any, error) {
		c.ApplySettings(app.Settings(p))
		return none{}, nil
	}
}

// fakeInject delivers a scripted message into a conversation, in test
// builds.
func fakeInject(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		return c.Inject(ctx, p.ConversationID)
	}
}
