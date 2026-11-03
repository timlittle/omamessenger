package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errSoundNotPlayable reports that a chosen file could not be
// transcoded into a sound this chat can play: ffmpeg itself did not
// recognise it as audio at all, or no transcoder was wired in (see
// storeAndTranscodeSound). Wrapped in ErrInvalidInput, its message is
// safe to show; it never names the file itself (see
// docs/decisions.md's sign-in hint entry, the same reasoning: a fixed
// sentence, not an echo of what the user gave), but it does name the
// formats that do work, so the refusal is actionable rather than just
// a dead end.
var errSoundNotPlayable = fmt.Errorf("%w: that file is not a sound this chat can play - try an MP3, WAV, Ogg, M4A or FLAC file", ErrInvalidInput)

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
// sound, transcoding path - a file the user picked from disk - into
// the helper's own sound store before this conversation is ever
// allowed to point at it: a user-chosen file is still untrusted bytes
// to whatever eventually decodes it, so nothing but this helper's own
// re-encoded WAV output is ever stored or played (see
// docs/decisions.md). Fails with ErrInvalidInput when path cannot be
// read or is not a format the transcoder recognises as audio.
func (c *Commands) SetNotificationSound(ctx context.Context, conversationID, path string) (domain.Conversation, error) {
	if conversationID == "" {
		return domain.Conversation{}, fmt.Errorf("%w: conversationId is required", ErrInvalidInput)
	}
	if path == "" {
		return domain.Conversation{}, fmt.Errorf("%w: a file is required", ErrInvalidInput)
	}

	if err := c.storeAndTranscodeSound(ctx, soundKeyConversation(conversationID), path); err != nil {
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
// or "" once it has no custom sound at all, or the stored file has
// gone missing since (see playableSoundPath).
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
	if err := c.storeAndTranscodeSound(ctx, key, path); err != nil {
		return ""
	}

	return c.sounds.Path(key)
}

// storeAndTranscodeSound transcodes path - a file the user picked, or
// a service's own manifest setting - into an uncompressed WAV through
// the sandboxed transcoder's Sound profile, the only format Qt's
// SoundEffect can play (see docs/decisions.md), and stores the result
// in the sound store under key: path itself is never what the sound
// store keeps, nor what resolve ever hands back. The transcode itself
// runs only when path's own size or modification time has actually
// changed since the last time this key was attempted (see
// transcodedSoundCache): resolveServiceSound calls this on every
// notification, and a service's default sound almost never changes,
// so without this a contact spamming messages would occupy the
// helper's one transcode slot on every single one, whether that
// sound keeps transcoding successfully or - a broken manifest setting,
// say - keeps failing the exact same way every time (see
// docs/decisions.md). errSoundNotPlayable reports path decoding as
// nothing ffmpeg itself recognises as audio, or no transcoder being
// wired in at all; any other error means path itself could not even
// be opened.
func (c *Commands) storeAndTranscodeSound(ctx context.Context, key, path string) error {
	if c.sounds == nil {
		return fmt.Errorf("%w: notification sounds are not available", ErrInvalidInput)
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	unlock := c.soundTranscodes.lock(key)
	defer unlock()

	if last, ok := c.soundTranscodes.lookup(key); ok && last.matches(path, info) {
		if last.failed {
			return errSoundNotPlayable
		}
		if _, statErr := os.Stat(c.sounds.Path(key)); statErr == nil {
			return nil
		}
	}
	if c.transcoder == nil {
		return errSoundNotPlayable
	}

	if err := c.transcodeSoundTo(ctx, key, path); err != nil {
		c.soundTranscodes.record(key, path, info, true)
		return errSoundNotPlayable
	}

	c.soundTranscodes.record(key, path, info, false)

	return nil
}

// transcodeSoundTo runs path through the sandboxed transcoder's Sound
// profile and copies its output into the sound store under key, the
// work storeAndTranscodeSound needs only when path has actually
// changed since the last time it ran for key.
func (c *Commands) transcodeSoundTo(ctx context.Context, key, path string) error {
	wav, err := c.transcoder.Sound(ctx, path, c.transcodeJobRoot)
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(wav))

	f, err := os.Open(wav)
	if err != nil {
		return err
	}
	defer f.Close() // reading only; nothing to flush

	_, err = c.sounds.Store(ctx, key, f)

	return err
}

// transcodedSoundIdentity is the (path, size, modification time) a
// sound-store key was last attempted from, and whether that attempt
// failed: resolveServiceSound calls storeAndTranscodeSound on every
// notification, so a source that fails to transcode - corrupt, not
// actually audio - must be remembered as failing too, or a broken
// custom or service sound would occupy the helper's one transcode
// slot on every single notification, not just its first one (see
// docs/decisions.md).
type transcodedSoundIdentity struct {
	path    string
	size    int64
	modTime int64
	failed  bool
}

// matches reports whether identity still describes path's current
// size and modification time.
func (identity transcodedSoundIdentity) matches(path string, info os.FileInfo) bool {
	return identity.path == path && identity.size == info.Size() && identity.modTime == info.ModTime().UnixNano()
}

// transcodedSoundCache remembers, per sound-store key, the identity
// of the source file last attempted for it, and serialises
// storeAndTranscodeSound per key (see lock) so two concurrent
// resolves for the same key - two notifications arriving close
// together, say - never both run the sandboxed transcoder for a sound
// neither has actually changed: whichever arrives second simply
// waits, then - almost always - finds the first call's own work
// already covers it.
type transcodedSoundCache struct {
	keysMu sync.Mutex
	keys   map[string]*sync.Mutex

	stateMu sync.Mutex
	state   map[string]transcodedSoundIdentity
}

// newTranscodedSoundCache returns an empty transcodedSoundCache.
func newTranscodedSoundCache() *transcodedSoundCache {
	return &transcodedSoundCache{keys: map[string]*sync.Mutex{}, state: map[string]transcodedSoundIdentity{}}
}

// lock reserves key's own lock, waiting for any other call already
// holding it, and returns the func that releases it. The key space
// here is small and chosen by the user, never a sender - one
// conversation id per custom sound, plus one per service - so this
// map simply grows with it rather than needing reference-counted
// cleanup.
func (t *transcodedSoundCache) lock(key string) func() {
	t.keysMu.Lock()
	l, ok := t.keys[key]
	if !ok {
		l = &sync.Mutex{}
		t.keys[key] = l
	}
	t.keysMu.Unlock()

	l.Lock()

	return l.Unlock
}

// lookup returns key's last recorded identity, and whether one exists
// at all.
func (t *transcodedSoundCache) lookup(key string) (transcodedSoundIdentity, bool) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	identity, ok := t.state[key]

	return identity, ok
}

// record remembers path's current identity as key's last transcode
// attempt, succeeded or not.
func (t *transcodedSoundCache) record(key, path string, info os.FileInfo, failed bool) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	t.state[key] = transcodedSoundIdentity{path: path, size: info.Size(), modTime: info.ModTime().UnixNano(), failed: failed}
}

// playableSoundPath returns key's stored path, or "" - never an
// error - once nothing is stored there, or it no longer exists on
// disk: a sound removed since it was stored simply plays nothing
// rather than failing the request that asked for it. The file is
// always a WAV this helper's own transcoder produced (see
// storeAndTranscodeSound), so there is nothing left to re-validate on
// a read the way an unchecked, sender-chosen file still would.
func (c *Commands) playableSoundPath(key string) string {
	path := c.sounds.Path(key)
	if _, err := os.Stat(path); err != nil {
		return ""
	}

	return path
}
