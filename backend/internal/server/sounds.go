package server

import (
	"context"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// soundMethods is methods' own notification sound entries, kept in a
// function of their own rather than growing methods' table literal
// further.
func soundMethods(c *app.Commands) map[string]method {
	return map[string]method{
		"conversations.setNotificationSound":   bind(conversationsSetNotificationSound(c)),
		"conversations.resetNotificationSound": bind(conversationsResetNotificationSound(c)),
		"sounds.fetch":                         bind(soundsFetch(c)),
	}
}

// notificationSoundParams names a conversation and the file the
// command palette's "Set notification sound…" asked to validate and
// store for it.
type notificationSoundParams struct {
	ConversationID string `json:"conversationId"`
	Path           string `json:"path"`
}

// conversationsSetNotificationSound gives a conversation its own
// notification sound, validating p.Path first; see
// app.Commands.SetNotificationSound.
func conversationsSetNotificationSound(c *app.Commands) func(context.Context, notificationSoundParams) (any, error) {
	return func(ctx context.Context, p notificationSoundParams) (any, error) {
		return c.SetNotificationSound(ctx, p.ConversationID, p.Path)
	}
}

// conversationsResetNotificationSound clears a conversation's own
// notification sound, for the command palette's "Reset notification
// sound"; see app.Commands.ResetNotificationSound.
func conversationsResetNotificationSound(c *app.Commands) func(context.Context, conversationParams) (any, error) {
	return func(ctx context.Context, p conversationParams) (any, error) {
		return c.ResetNotificationSound(ctx, p.ConversationID)
	}
}

// soundFetchParams asks for the playable path of a notification sound
// the UI has already decided should play (see
// ui/lib/NotificationSound.js): either ConversationID (that
// conversation's own custom sound) or Service and Path (a service's or
// the global default's manifest setting, named "whatsapp", "telegram"
// or "default"), never both.
type soundFetchParams struct {
	ConversationID string `json:"conversationId,omitempty"`
	Service        string `json:"service,omitempty"`
	Path           string `json:"path,omitempty"`
}

// soundFetchResult is sounds.fetch's result: Path, omitted when
// nothing playable was found, is a local file path the UI builds its
// own file:// URL from (see ui/lib/Links.js), never logged or placed
// in an event.
type soundFetchResult struct {
	Path string `json:"path,omitempty"`
}

// soundsFetch resolves a notification sound's playable path, validating
// it with audiosafe first; see app.Commands.ResolveNotificationSound.
func soundsFetch(c *app.Commands) func(context.Context, soundFetchParams) (any, error) {
	return func(ctx context.Context, p soundFetchParams) (any, error) {
		path, err := c.ResolveNotificationSound(ctx, p.ConversationID, p.Service, p.Path)

		return soundFetchResult{Path: path}, err
	}
}
