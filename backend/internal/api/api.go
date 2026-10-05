// Package api adapts the application commands to the rpc framing: it owns
// the C3 method table, params decoding and the error-to-code mapping, so
// neither rpc nor app needs to know about the other.
package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/rpc"
)

// Commands is the application surface the protocol exposes. It is declared
// here, by its consumer; *app.App satisfies it.
type Commands interface {
	DemoMode() bool
	Hello(context.Context, struct{}) (app.HelloResult, error)
	AccountsList(context.Context, struct{}) ([]domain.Account, error)
	ConversationsList(context.Context, app.ConversationsListParams) ([]domain.Conversation, error)
	MessagesList(context.Context, app.MessagesListParams) (app.MessagesListResult, error)
	SendMessage(context.Context, app.SendMessageParams) (domain.Message, error)
	Retry(context.Context, app.RetryParams) (domain.Message, error)
	MarkRead(context.Context, app.ConversationParams) (struct{}, error)
	SetMuted(context.Context, app.SetMutedParams) (domain.Conversation, error)
	OpenConversation(context.Context, app.OpenConversationParams) (domain.Conversation, error)
	ContactsList(context.Context, app.ContactsListParams) ([]domain.Contact, error)
	SetFocus(context.Context, app.FocusParams) (struct{}, error)
	ApplySettings(context.Context, app.SettingsParams) (struct{}, error)
	Inject(context.Context, app.InjectParams) (domain.Message, error)
}

// errInvalidParams reports params that do not decode into the method's type.
var errInvalidParams = errors.New("invalid params")

// Register builds the C3 method table. demo.inject exists only in demo mode;
// otherwise the framing layer answers it with unknown_method.
func Register(c Commands) rpc.Handler {
	h := rpc.Handler{
		"hello":                  bind(c.Hello),
		"accounts.list":          bind(c.AccountsList),
		"conversations.list":     bind(c.ConversationsList),
		"messages.list":          bind(c.MessagesList),
		"messages.send":          bind(c.SendMessage),
		"messages.retry":         bind(c.Retry),
		"conversations.markRead": bind(c.MarkRead),
		"conversations.setMuted": bind(c.SetMuted),
		"conversations.open":     bind(c.OpenConversation),
		"contacts.list":          bind(c.ContactsList),
		"ui.setFocus":            bind(c.SetFocus),
		"settings.apply":         bind(c.ApplySettings),
	}
	if c.DemoMode() {
		h["demo.inject"] = bind(c.Inject)
	}
	return h
}

func bind[P any, R any](fn func(context.Context, P) (R, error)) rpc.Method {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var params P
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, errInvalidParams
		}
		return fn(ctx, params)
	}
}

// Code maps an application error to a C3 error code. Bad-request messages
// describe the caller's mistake and are safe to return; anything unexpected
// becomes "internal" with a fixed message so internal details never leak.
func Code(err error) (code, message string) {
	switch {
	case errors.Is(err, errInvalidParams):
		return "bad_request", errInvalidParams.Error()
	case errors.Is(err, app.ErrBadRequest):
		return "bad_request", err.Error()
	case errors.Is(err, domain.ErrNotFound):
		return "not_found", "not found"
	case errors.Is(err, app.ErrUnknownMethod):
		return "unknown_method", "unknown method"
	default:
		return "internal", "internal error"
	}
}
