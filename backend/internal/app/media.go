package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

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
// then kept in the media cache. A download failure is always logged
// with its safe reason category and returned as a *MediaFetchError, so
// nothing fails silently (see docs/decisions.md).
func (c *Commands) FetchMedia(ctx context.Context, messageID string) (string, error) {
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

	return c.download(ctx, conv, m)
}

// download runs the cache-filling fetch for m and classifies a failure,
// logging it and wrapping it as a *MediaFetchError before it is
// returned. connectorErr, captured from the fill closure, is what tells
// a cache failure (the closure never even ran) apart from one the
// connector itself reported.
func (c *Commands) download(ctx context.Context, conv domain.Conversation, m domain.Message) (string, error) {
	var connectorErr error
	path, err := c.cache.Fetch(ctx, mediaFileName(m), func(ctx context.Context, path string) error {
		connectorErr = c.media.FetchMedia(ctx, conv, m.RemoteID, path)
		return connectorErr
	})
	if err == nil {
		return path, nil
	}

	reason := fetchFailureReason(connectorErr)
	c.logFetchFailure(conv.Service, m.Media.Kind, reason)

	return "", &MediaFetchError{Reason: reason, err: err}
}

// fetchFailureReason classifies a media download failure from the
// connector error the fill closure in download saw, or nil when the
// cache itself failed before or after calling the connector at all.
func fetchFailureReason(connectorErr error) MediaFetchReason {
	switch {
	case connectorErr == nil:
		return MediaCacheFailed
	case errors.Is(connectorErr, domain.ErrNotFound):
		return MediaNotFound
	case errors.Is(connectorErr, domain.ErrMediaExpired):
		return MediaExpired
	case errors.Is(connectorErr, domain.ErrMediaDecryptFailed):
		return MediaDecryptFailed
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

// mediaFileName names a message's media in the cache by the message's id,
// with an extension that says what it is, or for a file its own name, so
// it opens in the right application.
func mediaFileName(m domain.Message) string {
	switch m.Media.Kind {
	case domain.MediaPhoto:
		return m.ID + ".jpg"
	case domain.MediaVideo:
		return m.ID + ".mp4"
	}

	name := filepath.Base(m.Media.FileName)
	if name == "." || name == "/" {
		return m.ID
	}

	return m.ID + "-" + name
}
