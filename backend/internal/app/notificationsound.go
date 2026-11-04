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

// errSoundNotPlayable reports a chosen file could not be transcoded into
// a sound this chat can play. Its message is safe to show and never
// names the file itself.
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

// SetNotificationSound gives a conversation its own notification sound,
// transcoding path into the helper's sound store so only re-encoded WAV
// output is ever stored or played (see docs/decisions.md).
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

// ResetNotificationSound clears a conversation's custom notification
// sound, so it falls back to the service or global default, and
// removes the stored file.
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

// ResolveNotificationSound returns the playable path for a notification
// sound: either conversationID's own custom sound, or service and path
// from a manifest setting. An empty result with no error means nothing
// playable was found, so the UI plays nothing.
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

// resolveConversationSound returns conversationID's stored sound, or
// "" if it has none or the stored file is gone.
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

// resolveServiceSound transcodes and stores path under key, the same
// way SetNotificationSound does, but never reports an error: an
// invalid manifest setting just resolves to no sound.
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

// storeAndTranscodeSound transcodes path into a WAV and stores it under
// key, skipping the transcode when path hasn't changed since the last
// attempt for key (see transcodedSoundCache), so a flood of
// notifications never floods the transcoder.
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
// profile and copies its output into the sound store under key.
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
// failed, so a source that keeps failing to transcode is remembered
// rather than retried on every notification.
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

// transcodedSoundCache remembers, per sound-store key, the identity of
// the source file last attempted, and serialises storeAndTranscodeSound
// per key so two concurrent resolves never both transcode the same
// unchanged sound.
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
// holding it, and returns the func that releases it.
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

// playableSoundPath returns key's stored path, or "" if nothing is
// stored or the file no longer exists on disk.
func (c *Commands) playableSoundPath(key string) string {
	path := c.sounds.Path(key)
	if _, err := os.Stat(path); err != nil {
		return ""
	}

	return path
}
