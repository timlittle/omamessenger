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
		"hello":                     bind(hello(c, version)),
		"accounts.list":             bind(accountsList(c)),
		"accounts.add":              bind(accountsAdd(c)),
		"accounts.remove":           bind(accountsRemove(c)),
		"auth.submit":               bind(authSubmit(c)),
		"contacts.list":             bind(contactsList(c)),
		"conversations.list":        bind(conversationsList(c)),
		"conversations.open":        bind(conversationsOpen(c)),
		"conversations.markRead":    bind(conversationsMarkRead(c)),
		"conversations.setMuted":    bind(conversationsSetMuted(c)),
		"conversations.setPinned":   bind(conversationsSetPinned(c)),
		"conversations.setArchived": bind(conversationsSetArchived(c)),
		"conversations.setHidden":   bind(conversationsSetHidden(c)),
		"conversations.setReminder": bind(conversationsSetReminder(c)),
		"conversations.members":     bind(conversationsMembers(c)),
		"messages.list":             bind(messagesList(c)),
		"messages.send":             bind(messagesSend(c)),
		"messages.retry":            bind(messagesRetry(c)),
		"messages.react":            bind(messagesReact(c)),
		"messages.delete":           bind(messagesDelete(c)),
		"media.fetch":               bind(mediaFetch(c)),
		"media.paste":               bind(mediaPaste(c)),
		"ui.setFocus":               bind(uiSetFocus(c)),
		"settings.apply":            bind(settingsApply(c)),
		"helper.doctor":             bind(helperDoctor(c)),
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

// pinnedParams pins or unpins a conversation.
type pinnedParams struct {
	ConversationID string `json:"conversationId"`
	Pinned         bool   `json:"pinned"`
}

// conversationsSetPinned pins or unpins a conversation.
func conversationsSetPinned(c *app.Commands) func(context.Context, pinnedParams) (any, error) {
	return func(ctx context.Context, p pinnedParams) (any, error) {
		return c.SetPinned(ctx, p.ConversationID, p.Pinned)
	}
}

// archivedParams archives or unarchives a conversation.
type archivedParams struct {
	ConversationID string `json:"conversationId"`
	Archived       bool   `json:"archived"`
}

// conversationsSetArchived archives or unarchives a conversation.
func conversationsSetArchived(c *app.Commands) func(context.Context, archivedParams) (any, error) {
	return func(ctx context.Context, p archivedParams) (any, error) {
		return c.SetArchived(ctx, p.ConversationID, p.Archived)
	}
}

// hiddenParams hides or unhides a conversation.
type hiddenParams struct {
	ConversationID string `json:"conversationId"`
	Hidden         bool   `json:"hidden"`
}

// conversationsSetHidden hides or unhides a conversation. Hiding is local
// to this computer only and is never reported to the service.
func conversationsSetHidden(c *app.Commands) func(context.Context, hiddenParams) (any, error) {
	return func(ctx context.Context, p hiddenParams) (any, error) {
		return c.SetHidden(ctx, p.ConversationID, p.Hidden)
	}
}

// reminderParams snoozes or unsnoozes a conversation. At is milliseconds
// since the Unix epoch to snooze until, or nil to clear it, matching the
// protocol's {conversationId, at | null} shape.
type reminderParams struct {
	ConversationID string `json:"conversationId"`
	At             *int64 `json:"at"`
}

// conversationsSetReminder snoozes a conversation until At, or clears its
// reminder when At is nil. Snoozing is local to this computer only and is
// never reported to the service.
func conversationsSetReminder(c *app.Commands) func(context.Context, reminderParams) (any, error) {
	return func(ctx context.Context, p reminderParams) (any, error) {
		var at int64
		if p.At != nil {
			at = *p.At
		}

		return c.SetReminder(ctx, p.ConversationID, at)
	}
}

// membersResult is a group's current members, for the @-mention picker.
type membersResult struct {
	Members []domain.Member `json:"members"`
}

// conversationsMembers lists a conversation's current members.
func conversationsMembers(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		members, err := c.Members(ctx, p.ConversationID)

		return membersResult{Members: members}, err
	}
}

// messagesParams selects a page of messages.
type messagesParams struct {
	ConversationID string `json:"conversationId"`
	Before         string `json:"before"`
	Limit          int    `json:"limit"`
}

// messagesResult is a page of messages, oldest first. HistoryUnavailable
// is set when paging past the oldest stored message asked the service for
// more and could not reach it right now (see app.Commands.Messages); an
// older UI that does not read this field keeps working unchanged.
type messagesResult struct {
	Messages           []domain.Message `json:"messages"`
	HasMore            bool             `json:"hasMore"`
	HistoryUnavailable bool             `json:"historyUnavailable,omitempty"`
}

// messagesList returns a page of a conversation's messages.
func messagesList(c *app.Commands) func(context.Context, messagesParams) (any, error) {
	return func(ctx context.Context, p messagesParams) (any, error) {
		page, more, unavailable, err := c.Messages(ctx, p.ConversationID, p.Before, p.Limit)
		return messagesResult{Messages: page, HasMore: more, HistoryUnavailable: unavailable}, err
	}
}

// sendParams is a message to send. Attachment names a file on this
// machine to send with text as its caption; it is optional, so an older
// UI sending just conversationId and text keeps working unchanged.
// ReplyTo, when set, is the local id of a message in the same
// conversation this one answers. Mentions, also optional, are the
// "@name" tokens the composer inserted into text.
type sendParams struct {
	ConversationID string            `json:"conversationId"`
	Text           string            `json:"text"`
	Attachment     *attachmentParams `json:"attachment,omitempty"`
	ReplyTo        string            `json:"replyTo,omitempty"`
	Mentions       []domain.Mention  `json:"mentions,omitempty"`
}

// attachmentParams names a file to attach to an outgoing message.
type attachmentParams struct {
	Path string `json:"path"`
}

// messagesSend sends a message.
func messagesSend(c *app.Commands) func(context.Context, sendParams) (any, error) {
	return func(ctx context.Context, p sendParams) (any, error) {
		return c.Send(ctx, p.ConversationID, p.Text, app.SendOptions{
			AttachmentPath: attachmentPath(p.Attachment), ReplyToID: p.ReplyTo, Mentions: p.Mentions,
		})
	}
}

// attachmentPath is the file path an attachment names, or "" when a
// request carries none.
func attachmentPath(a *attachmentParams) string {
	if a == nil {
		return ""
	}

	return a.Path
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

// reactParams sets or clears the user's reaction to a message; emoji ""
// clears it.
type reactParams struct {
	MessageID string `json:"messageId"`
	Emoji     string `json:"emoji"`
}

// messagesReact sets or clears the user's reaction to a message.
func messagesReact(c *app.Commands) func(context.Context, reactParams) (any, error) {
	return func(ctx context.Context, p reactParams) (any, error) {
		return c.React(ctx, p.MessageID, p.Emoji)
	}
}

// deleteParams names the messages to delete from a conversation: for
// everyone, through the service, when forEveryone is true, or only for
// this account otherwise.
type deleteParams struct {
	ConversationID string   `json:"conversationId"`
	MessageIDs     []string `json:"messageIds"`
	ForEveryone    bool     `json:"forEveryone"`
}

// messagesDelete deletes messages from a conversation.
func messagesDelete(c *app.Commands) func(context.Context, deleteParams) (any, error) {
	return func(ctx context.Context, p deleteParams) (any, error) {
		return none{}, c.DeleteMessages(ctx, p.ConversationID, p.MessageIDs, p.ForEveryone)
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

// pasteResult is the image media.paste found on the clipboard, copied
// into the outgoing media area.
type pasteResult struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// mediaPaste copies an image off the clipboard, for the composer to
// attach to the next message sent, or fails with an invalid-input error
// when the clipboard holds no image.
func mediaPaste(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		img, err := c.PasteImage(ctx)

		return pasteResult{Path: img.Path, Kind: img.Kind, Width: img.Width, Height: img.Height}, err
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

// settingsParams are the plugin settings. NotificationDetail is additive:
// an older UI that sends only NotificationPreview still works, since
// app.Settings falls back to it when NotificationDetail is empty.
type settingsParams struct {
	Notifications       bool   `json:"notifications"`
	NotificationPreview bool   `json:"notificationPreview"`
	NotificationDetail  string `json:"notificationDetail"`
}

// settingsApply replaces the user's settings.
func settingsApply(c *app.Commands) func(context.Context, settingsParams) (any, error) {
	return func(_ context.Context, p settingsParams) (any, error) {
		c.ApplySettings(app.Settings(p))
		return none{}, nil
	}
}

// helperDoctor returns the helper's own health report: safe states and
// categories only, nothing that identifies the user or their accounts.
func helperDoctor(c *app.Commands) func(context.Context, none) (any, error) {
	return func(ctx context.Context, _ none) (any, error) {
		return c.Doctor(ctx)
	}
}

// fakeInject delivers a scripted message into a conversation, in test
// builds.
func fakeInject(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		return c.Inject(ctx, p.ConversationID)
	}
}
