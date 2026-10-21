package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// MediaFetchReason categorizes why a message's media could not be
// downloaded. The categories are coarse on purpose: they are safe to
// write to the helper's log and to send to the UI, unlike the error
// underneath, which may name a server, a file path or a protocol detail
// that must stay in the helper (see errors.md).
type MediaFetchReason string

// The reasons FetchMedia reports, from the most to the least specific.
// MediaDownloadFailed is the fallback for anything that is none of the
// others: a network failure, a server error, or a reference the service
// no longer recognises.
const (
	MediaNotFound       MediaFetchReason = "not-found"
	MediaExpired        MediaFetchReason = "expired"
	MediaDecryptFailed  MediaFetchReason = "decrypt"
	MediaTooLarge       MediaFetchReason = "too-large"
	MediaCacheFailed    MediaFetchReason = "cache"
	MediaTimedOut       MediaFetchReason = "timeout"
	MediaDownloadFailed MediaFetchReason = "download"
)

// MediaFetchError reports that FetchMedia failed, together with the safe
// reason category above: the server package reads Reason to hand the UI
// something to show without any other detail from Unwrap's error ever
// crossing the protocol.
type MediaFetchError struct {
	Reason MediaFetchReason
	err    error
}

// Error returns the wrapped error's own message, for the helper's own
// log and stderr; the UI never sees this text (see rpcError).
func (e *MediaFetchError) Error() string {
	return e.err.Error()
}

// Unwrap exposes the underlying error, so errors.Is and errors.As still
// see through a MediaFetchError to what actually failed.
func (e *MediaFetchError) Unwrap() error {
	return e.err
}

// FetchMedia returns the path of a message's photo, video, file or voice
// note. An outgoing message's own attachment is already on this
// machine, so its local copy is returned at once, with no remote id
// needed; anything else is downloaded from the service the first time,
// then kept in the media cache. auto is true for a fetch the UI made on
// its own, with no click from the user - today, only an animated "GIF"
// video autoplaying as soon as it is shown - which every connector
// bounds to a far smaller size than a deliberate click is trusted
// with (see connector.MaxBytes); SaveMedia, always a deliberate action,
// passes false. A download failure is always logged with its safe
// reason category and returned as a *MediaFetchError, so nothing fails
// silently (see docs/decisions.md).
func (c *Commands) FetchMedia(ctx context.Context, messageID string, auto bool) (string, error) {
	if strings.TrimSpace(messageID) == "" {
		return "", fmt.Errorf("%w: messageId is required", ErrInvalidInput)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return "", err
	}

	if m.Media == nil || m.Media.Kind == domain.MediaLink {
		return "", fmt.Errorf("%w: this message has nothing to download", ErrInvalidInput)
	}

	if path, ok := c.localAttachment(m); ok {
		return path, nil
	}

	if m.RemoteID == "" || c.media == nil || c.cache == nil {
		return "", fmt.Errorf("%w: this message has nothing to download", ErrInvalidInput)
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return "", err
	}

	return c.download(ctx, conv, m, auto)
}

// DecodeAsForMedia reports how the UI must decode messageID's video,
// already downloaded to path by a prior FetchMedia: "gif" once path's
// own bytes are confirmed, by their magic header alone, to be a
// genuine GIF image, sanitized through imagesafe first, in place, the
// same check and profile every other image this UI ever shows must
// pass (see connector.SanitizeIfGIF); "" for an ordinary video, or any
// other media kind, which the UI must never decode as an animated
// image. Never inferred from a sender's filename, mime type or
// "animated" flag - the bug this replaces - so the result can only
// ever disagree with that metadata in the safe direction.
func (c *Commands) DecodeAsForMedia(ctx context.Context, messageID, path string) (string, error) {
	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return "", err
	}
	if m.Media == nil || m.Media.Kind != domain.MediaVideo {
		return "", nil
	}

	isGIF, err := connector.SanitizeIfGIF(path)
	if err != nil {
		return "", c.failDecodeCheck(ctx, m, err)
	}
	if !isGIF {
		return "", nil
	}

	return "gif", nil
}

// failDecodeCheck classifies and logs a SanitizeIfGIF failure the same
// way download classifies and logs a failed fetch, so a video that
// turned out to be unsafe once its actual bytes were checked is never
// silently left as it was.
func (c *Commands) failDecodeCheck(ctx context.Context, m domain.Message, err error) error {
	conv, convErr := c.store.Conversation(ctx, m.ConversationID)
	if convErr != nil {
		return convErr
	}

	reason := fetchFailureReason(err, err)
	c.logFetchFailure(conv.Service, m.Media.Kind, reason)

	return &MediaFetchError{Reason: reason, err: err}
}

// download runs the cache-filling fetch for m and classifies a failure,
// logging it and wrapping it as a *MediaFetchError before it is
// returned. connectorErr, captured from the fill closure, is what tells
// a cache failure (the closure never even ran, or the cache itself
// refused the result afterwards - see cache.ErrTooLargeForCache) apart
// from one the connector itself reported.
func (c *Commands) download(ctx context.Context, conv domain.Conversation, m domain.Message, auto bool) (string, error) {
	var connectorErr error
	path, err := c.fetchIntoCache(ctx, m, func(ctx context.Context, path string) error {
		connectorErr = c.media.FetchMedia(ctx, conv, m.RemoteID, path, auto)
		return connectorErr
	})
	if err == nil {
		return path, nil
	}

	reason := fetchFailureReason(err, connectorErr)
	c.logFetchFailure(conv.Service, m.Media.Kind, reason)

	return "", &MediaFetchError{Reason: reason, err: err}
}

// fetchIntoCache fills the media cache for m: Fetch under a fixed, safe
// name for a photo or video (see fixedMediaName), never chosen from
// anything the sender claims, or FetchNamed's own sniffed one for
// every other kind, chosen only once fill's downloaded bytes exist, so
// a file, voice note or sticker's cache name is never again picked
// from the sender's own file name the way it used to be (see
// docs/decisions.md).
func (c *Commands) fetchIntoCache(ctx context.Context, m domain.Message, fill func(context.Context, string) error) (string, error) {
	if name, ok := fixedMediaName(m); ok {
		return c.cache.Fetch(ctx, name, fill)
	}

	return c.cache.FetchNamed(ctx, m.ID, voiceHintedFill(m, fill), func(part string) (string, error) {
		contentType, err := sniffContentType(part)
		if err != nil {
			return "", err
		}

		return m.ID + extensionFor(contentType), nil
	})
}

// voiceHintedFill wraps fill so a voice note's download briefly exists
// under a path ending ".ogg" before being moved to the plain one
// FetchNamed actually asked for, once fill has written it: the fake
// connector `make build-fake` builds for tests decides which
// placeholder bytes to write from path's own extension, the one thing
// it can tell a voice note apart from a photo by without the message's
// media kind in its own signature (see fake.Connector.FetchMedia,
// matching every real connector's). Every real connector ignores
// path's extension and writes whatever the service actually sent
// regardless, so this changes nothing for them; it never changes the
// file's own final cache name either, which fetchIntoCache's own
// nameFor always decides from the downloaded bytes, never this hint.
func voiceHintedFill(m domain.Message, fill func(context.Context, string) error) func(context.Context, string) error {
	if m.Media.Kind != domain.MediaVoice {
		return fill
	}

	return func(ctx context.Context, path string) error {
		hinted := path + ".ogg"
		if err := fill(ctx, hinted); err != nil {
			if rmErr := os.Remove(hinted); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				return errors.Join(err, rmErr)
			}

			return err
		}

		return os.Rename(hinted, path)
	}
}

// fetchFailureReason classifies a media download failure: err is what
// cache.Fetch itself returned, which also catches a result the cache
// refused after a successful download (too large for its own limit);
// connectorErr is what the fill closure in download saw, or nil when
// the cache failed before ever calling the connector at all.
func fetchFailureReason(err, connectorErr error) MediaFetchReason {
	if errors.Is(err, cache.ErrTooLargeForCache) {
		return MediaTooLarge
	}

	switch {
	case connectorErr == nil:
		return MediaCacheFailed
	case errors.Is(connectorErr, domain.ErrNotFound):
		return MediaNotFound
	case errors.Is(connectorErr, domain.ErrMediaExpired):
		return MediaExpired
	case errors.Is(connectorErr, domain.ErrMediaDecryptFailed):
		return MediaDecryptFailed
	case errors.Is(connectorErr, connector.ErrMediaTooLarge):
		return MediaTooLarge
	case errors.Is(connectorErr, context.DeadlineExceeded), errors.Is(connectorErr, context.Canceled):
		return MediaTimedOut
	default:
		return MediaDownloadFailed
	}
}

// logFetchFailure writes one diagnostic line for a failed media
// download: the service and the message's media kind, which are never
// private, and the safe reason category, never the error itself, which
// could otherwise echo a server detail or a file path into the log. It
// also records the category for Doctor to report, alongside the log line
// that already carries it.
func (c *Commands) logFetchFailure(service, kind string, reason MediaFetchReason) {
	c.recentErrors.record(string(reason))

	if c.logger == nil {
		return
	}

	c.logger.Printf("media: fetch failed (service=%s, kind=%s, reason=%s)", service, kind, reason)
}

// errorHistoryLimit bounds how many categories errorHistory keeps,
// oldest dropped first.
const errorHistoryLimit = 5

// errorHistory keeps the last few safe error categories recorded during
// this run, for Doctor to report. It holds only the same category words
// already safe to log, never a path, a name or a token.
type errorHistory struct {
	mu    sync.Mutex
	items []string
}

// record appends category, dropping the oldest once past errorHistoryLimit.
func (h *errorHistory) record(category string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.items = append(h.items, category)
	if len(h.items) > errorHistoryLimit {
		h.items = h.items[len(h.items)-errorHistoryLimit:]
	}
}

// snapshot returns a copy of the categories recorded so far, oldest first.
func (h *errorHistory) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return slices.Clone(h.items)
}

// localAttachment returns the path of an outgoing message's own
// attachment, already copied into the outgoing media area when it was
// sent, so the UI never has to download a photo back from the service it
// was just uploaded to. It reports false for an incoming message, or an
// outgoing one whose local copy is gone, so the caller falls back to
// downloading it.
func (c *Commands) localAttachment(m domain.Message) (string, bool) {
	if !m.Outgoing || c.outgoing == nil {
		return "", false
	}

	path := c.outgoing.Path(m.ID, m.Media.FileName)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}

	return path, true
}

// fixedMediaName returns the cache name a photo or video always uses,
// and true: an extension that says what it is, never the sender's
// own filename or attributes, which fetchIntoCache must trust even
// less for these two kinds than for any other, since Qt's own image
// and video engines are handed this file directly by its extension.
// Every MediaVideo message is cached as ".mp4", even the rare one
// whose downloaded bytes turn out to be a genuine GIF image once
// DecodeAsForMedia checks them: Qt's own image plugins sniff a file's
// actual format from its content, not its extension (see
// DecodeAsForMedia and docs/decisions.md). ok is false for every other
// kind, whose correct name can only be known once its own bytes exist
// to sniff (see fetchIntoCache).
func fixedMediaName(m domain.Message) (name string, ok bool) {
	switch m.Media.Kind {
	case domain.MediaPhoto:
		return m.ID + ".jpg", true
	case domain.MediaVideo:
		return m.ID + ".mp4", true
	default:
		return "", false
	}
}
