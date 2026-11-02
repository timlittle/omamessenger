package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/audiosafe"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errSoundNotPlayable reports that a chosen file's own bytes do not
// describe one of the formats audiosafe.Validate accepts: Ogg/Opus or
// M4A/AAC (MP3 is never accepted, however it is tagged - see
// audiosafe's own package doc comment and docs/decisions.md). Wrapped
// in ErrInvalidInput, its message is safe to show; it never names the
// file itself (see docs/decisions.md's sign-in hint entry, the same
// reasoning: a fixed sentence, not an echo of what the user gave), but
// it does name the formats that do work, so the refusal is actionable
// rather than just a dead end.
var errSoundNotPlayable = fmt.Errorf("%w: that file is not a sound this chat can play - try an Ogg/Opus or M4A file", ErrInvalidInput)

// soundKeyConversation is the sound store key a conversation's own
// custom notification sound is kept under.
func soundKeyConversation(conversationID string) string {
	return "conversation-" + conversationID
}

// soundKeyService is the sound store key a service's, or the global
// default's, own notification sound is kept under; service is
// "whatsapp", "telegram" or "default".
func soundKeyService(service string) string {
	return "service-" + service
}

// SetNotificationSound gives a conversation its own notification
// sound, copying path - a file the user picked from disk - into the
// helper's own sound store and confirming the copy with audiosafe
// before this conversation is ever allowed to point at it: a
// user-chosen file is still untrusted bytes to whatever eventually
// decodes it (see docs/decisions.md). Fails with ErrInvalidInput when
// path cannot be read or is not a format audiosafe recognises.
func (c *Commands) SetNotificationSound(ctx context.Context, conversationID, path string) (domain.Conversation, error) {
	if conversationID == "" {
		return domain.Conversation{}, fmt.Errorf("%w: conversationId is required", ErrInvalidInput)
	}
	if path == "" {
		return domain.Conversation{}, fmt.Errorf("%w: a file is required", ErrInvalidInput)
	}

	if err := c.storeAndValidateSound(ctx, soundKeyConversation(conversationID), path); err != nil {
		if errors.Is(err, errSoundNotPlayable) {
			return domain.Conversation{}, err
		}

		return domain.Conversation{}, fmt.Errorf("%w: could not read that file", ErrInvalidInput)
	}

	if err := c.store.SetNotificationSound(ctx, conversationID, true); err != nil {
		return domain.Conversation{}, err
	}

	return c.store.Conversation(ctx, conversationID)
}

// ResetNotificationSound clears a conversation's own notification
// sound, so it goes back to inheriting whichever one applies at the
// service or global default level, and removes the file
// SetNotificationSound copied in for it.
func (c *Commands) ResetNotificationSound(ctx context.Context, conversationID string) (domain.Conversation, error) {
	if conversationID == "" {
		return domain.Conversation{}, fmt.Errorf("%w: conversationId is required", ErrInvalidInput)
	}

	if err := c.store.SetNotificationSound(ctx, conversationID, false); err != nil {
		return domain.Conversation{}, err
	}

	if c.sounds != nil {
		if err := c.sounds.Remove(ctx, soundKeyConversation(conversationID)); err != nil {
			return domain.Conversation{}, err
		}
	}

	return c.store.Conversation(ctx, conversationID)
}

// ResolveNotificationSound returns the playable, already-validated path
// for a notification sound the UI has decided should play: exactly one
// of conversationID (the conversation's own custom sound) or service
// and path (a service's or the global default's manifest setting,
// still just a raw string the UI forwarded, validated here on its
// behalf) is given. An empty result with no error means nothing
// playable was found - removed since, say, or a manifest path that no
// longer resolves to a real sound - so the UI plays nothing rather than
// treating a notification sound as worth failing over.
func (c *Commands) ResolveNotificationSound(ctx context.Context, conversationID, service, path string) (string, error) {
	switch {
	case conversationID != "":
		return c.resolveConversationSound(ctx, conversationID)
	case service != "":
		return c.resolveServiceSound(ctx, service, path), nil
	default:
		return "", fmt.Errorf("%w: conversationId or service is required", ErrInvalidInput)
	}
}

// resolveConversationSound returns conversationID's own stored sound,
// re-validated here rather than trusted from when SetNotificationSound
// first stored it, or "" once it has no custom sound at all.
func (c *Commands) resolveConversationSound(ctx context.Context, conversationID string) (string, error) {
	conv, err := c.store.Conversation(ctx, conversationID)
	if err != nil {
		return "", err
	}
	if conv.NotificationSound != "custom" || c.sounds == nil {
		return "", nil
	}

	return c.playableSoundPath(soundKeyConversation(conversationID)), nil
}

// resolveServiceSound copies and validates path - a service's or the
// global default's manifest setting - under key, the same way
// SetNotificationSound does for a conversation, but never reports an
// error: an invalid manifest setting has no user action to fail here,
// so it simply resolves to no sound.
func (c *Commands) resolveServiceSound(ctx context.Context, service, path string) string {
	if path == "" || c.sounds == nil {
		return ""
	}

	key := soundKeyService(service)
	if err := c.storeAndValidateSound(ctx, key, path); err != nil {
		return ""
	}

	return c.sounds.Path(key)
}

// storeAndValidateSound copies path's bytes into the sound store under
// key and confirms the copy with audiosafe.Validate, removing it again
// and reporting errSoundNotPlayable when it does not decode as one of
// the formats audiosafe recognises; any other error means path itself
// could not be read.
func (c *Commands) storeAndValidateSound(ctx context.Context, key, path string) error {
	if c.sounds == nil {
		return fmt.Errorf("%w: notification sounds are not available", ErrInvalidInput)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() // reading only; nothing to flush

	dst, err := c.sounds.Store(ctx, key, f)
	if err != nil {
		return err
	}

	playable, err := audiosafe.Validate(dst)
	if err != nil {
		return err
	}
	if !playable {
		if removeErr := c.sounds.Remove(ctx, key); removeErr != nil {
			return removeErr
		}

		return errSoundNotPlayable
	}

	return nil
}

// playableSoundPath returns key's stored path once audiosafe.Validate
// confirms it still decodes, or "" - never an error - once it does not:
// a sound gone missing or no longer valid since it was stored simply
// plays nothing rather than failing the request that asked for it.
func (c *Commands) playableSoundPath(key string) string {
	path := c.sounds.Path(key)

	playable, err := audiosafe.Validate(path)
	if err != nil || !playable {
		return ""
	}

	return path
}
