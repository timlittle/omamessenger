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

	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
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

// FetchMedia returns the path of a message's photo, video, file or
// voice note, with width and height for a photo or sticker (zero
// otherwise), plus the kind and size actually served. auto marks a
// fetch the UI made on its own rather than from a user click, and play
// also requests an in-window play copy via playCopy, whose failure
// never fails the fetch. A download failure is logged with its safe
// reason category and returned as a *MediaFetchError.
func (c *Commands) FetchMedia(ctx context.Context, messageID string, auto, play bool) (path, playPath string, width, height int, kind string, mediaSize int64, err error) {
	if strings.TrimSpace(messageID) == "" {
		return "", "", 0, 0, "", 0, fmt.Errorf("%w: messageId is required", ErrInvalidInput)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return "", "", 0, 0, "", 0, err
	}

	if m.Media == nil || m.Media.Kind == domain.MediaLink {
		return "", "", 0, 0, "", 0, fmt.Errorf("%w: this message has nothing to download", ErrInvalidInput)
	}
	kind, mediaSize = m.Media.Kind, m.Media.Size

	if reason, ok := c.refused.lookup(cacheIdentity(m)); ok {
		return "", "", 0, 0, kind, mediaSize, &MediaFetchError{Reason: reason, err: errRefusedDisplayCopyCached}
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return "", "", 0, 0, kind, mediaSize, err
	}

	if local, ok := c.localAttachment(m); ok {
		path, width, height, err = c.servedPath(ctx, conv, m, local, false)
	} else {
		path, width, height, err = c.fetchAndServe(ctx, conv, m, auto)
	}
	if err != nil {
		return "", "", 0, 0, kind, mediaSize, err
	}

	if play {
		playPath = c.playCopy(ctx, m, path)
	}

	return path, playPath, width, height, kind, mediaSize, nil
}

// fetchAndServe downloads m's media when nothing local exists yet,
// then runs the result through servedPath like any other path.
func (c *Commands) fetchAndServe(ctx context.Context, conv domain.Conversation, m domain.Message, auto bool) (path string, width, height int, err error) {
	if m.RemoteID == "" || c.media == nil || c.cache == nil {
		return "", 0, 0, fmt.Errorf("%w: this message has nothing to download", ErrInvalidInput)
	}

	downloaded, err := c.download(ctx, conv, m, auto)
	if err != nil {
		return "", 0, 0, err
	}

	return c.servedPath(ctx, conv, m, downloaded, true)
}

// imageSafeProfile returns the imagesafe.Profile a message of kind
// must pass before FetchMedia hands its file to the UI, and true: the
// full-size profile for a photo, the thumbnail one for a sticker. ok
// is false for video, voice and audio, which get their play copy from
// playCopy instead.
func imageSafeProfile(kind string) (profile imagesafe.Profile, ok bool) {
	switch kind {
	case domain.MediaPhoto:
		return imagesafe.FullImage, true
	case domain.MediaSticker:
		return imagesafe.Thumbnail, true
	default:
		return imagesafe.Profile{}, false
	}
}

// servedPath turns the original bytes at path into the display copy a
// photo or sticker must pass before FetchMedia returns it; every other
// kind passes path straight through. refetchable allows
// recoverUnsafeImage one retry from the service when that copy fails
// to build; it is false for an outgoing message's own local copy.
func (c *Commands) servedPath(ctx context.Context, conv domain.Conversation, m domain.Message, path string, refetchable bool) (string, int, int, error) {
	profile, ok := imageSafeProfile(m.Media.Kind)
	if !ok {
		return path, 0, 0, nil
	}

	view, width, height, err := c.tryDisplayCopy(ctx, m, path, profile)
	if err != nil {
		return c.recoverUnsafeImage(ctx, conv, m, err, refetchable)
	}

	return view, width, height, nil
}

// tryDisplayCopy builds m's display copy from originalPath, removing
// originalPath only when imagesafe refused its bytes outright: a file
// that cannot be shown safely is never left trusted on disk. Any other
// failure leaves originalPath alone, since it says nothing about the
// bytes themselves.
func (c *Commands) tryDisplayCopy(ctx context.Context, m domain.Message, originalPath string, profile imagesafe.Profile) (path string, width, height int, err error) {
	path, width, height, err = c.displayCopy(ctx, m, originalPath, profile)
	if err == nil {
		return path, width, height, nil
	}
	if !errors.Is(err, connector.ErrUnsafeImage) {
		return "", 0, 0, err
	}

	if removeErr := os.Remove(originalPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return "", 0, 0, errors.Join(err, removeErr)
	}

	return "", 0, 0, err
}

// displayCopyID names m's slot for its display copy in the media
// cache, distinct from cacheIdentity's slot for the original bytes.
// The "-view" suffix (not ".view") keeps it from matching as a prefix
// of cacheIdentity(m) in cache.findByID's own lookup.
func displayCopyID(m domain.Message) string {
	return cacheIdentity(m) + "-view"
}

// displayCopy returns the path of m's display copy, re-encoding
// originalPath once under profile and serving any later cache hit as
// is, since re-encoding is lossy and must not repeat on every scroll.
// width and height come from the result's own header on every call,
// which is cheap compared to a full decode.
func (c *Commands) displayCopy(ctx context.Context, m domain.Message, originalPath string, profile imagesafe.Profile) (path string, width, height int, err error) {
	id := displayCopyID(m)
	var format imagesafe.OutputFormat

	fill := func(ctx context.Context, dst string) error {
		f, fillErr := c.reencodeForDisplay(ctx, originalPath, dst, profile)
		format = f
		return fillErr
	}
	nameFor := func(string) (string, error) {
		return id + format.Extension(), nil
	}

	path, err = c.cache.FetchNamed(ctx, id, fill, nameFor)
	if err != nil {
		return "", 0, 0, fmt.Errorf("media: display copy: %w", err)
	}

	width, height, ok := dimensionsOf(path)
	if !ok {
		return "", 0, 0, fmt.Errorf("media: display copy: %w", connector.ErrUnsafeImage)
	}

	return path, width, height, nil
}

// reencodeForDisplay is displayCopy's fill: the ordinary re-encode,
// falling back to the sandboxed transcoder's StillFrame (then a
// second re-encode of that frame) only when the ordinary re-encode
// refuses the bytes and a transcoder is wired in. This lets formats Go
// can't decode at all, such as an animated WebP sticker, still get a
// display copy via ffmpeg.
func (c *Commands) reencodeForDisplay(ctx context.Context, originalPath, dst string, profile imagesafe.Profile) (imagesafe.OutputFormat, error) {
	format, err := c.reencodeImageFile(ctx, originalPath, dst, profile)
	if err == nil {
		return format, nil
	}
	if !errors.Is(err, connector.ErrUnsafeImage) || c.transcoder == nil {
		return imagesafe.FormatUnknown, err
	}

	frame, frameErr := c.transcoder.StillFrame(ctx, originalPath, c.transcodeJobRoot)
	if frameErr != nil {
		return imagesafe.FormatUnknown, err // the original refusal is what this profile's caller needs to see, not the fallback's own
	}
	defer os.RemoveAll(filepath.Dir(frame))

	format, stillErr := c.reencodeImageFile(ctx, frame, dst, profile)
	if stillErr != nil {
		return imagesafe.FormatUnknown, err
	}

	return format, nil
}

// dimensionsOf reads width and height from path's header, without a
// full decode.
func dimensionsOf(path string) (width, height int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close() // reading only; nothing to lose on a close error

	return imagesafe.Dimensions(f)
}

// recoverUnsafeImage retries a download once, for a failed display
// copy, since tryDisplayCopy already removed the bad original. An
// outgoing message's own copy, or a retry that fails again, is
// classified and logged via failDecodeCheck instead.
func (c *Commands) recoverUnsafeImage(ctx context.Context, conv domain.Conversation, m domain.Message, checkErr error, refetchable bool) (string, int, int, error) {
	if !refetchable {
		return "", 0, 0, c.failDecodeCheck(ctx, m, checkErr)
	}

	path, err := c.download(ctx, conv, m, false)
	if err != nil {
		return "", 0, 0, err
	}

	profile, _ := imageSafeProfile(m.Media.Kind)
	view, width, height, err := c.tryDisplayCopy(ctx, m, path, profile)
	if err != nil {
		return "", 0, 0, c.failDecodeCheck(ctx, m, err)
	}

	return view, width, height, nil
}

// failDecodeCheck classifies and logs a failed display-copy check the
// same way download does, and remembers m under c.refused so a later
// FetchMedia fails fast instead of re-downloading and re-decoding on
// every scroll.
func (c *Commands) failDecodeCheck(ctx context.Context, m domain.Message, err error) error {
	conv, convErr := c.store.Conversation(ctx, m.ConversationID)
	if convErr != nil {
		return convErr
	}

	reason := fetchFailureReason(err, err)
	c.logFetchFailure(conv.Service, m.Media.Kind, reason)
	c.refused.remember(cacheIdentity(m), reason)

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

// fetchIntoCache fills the media cache for m: a fixed, safe name for a
// photo or video (see fixedMediaName), or a name sniffed from the
// downloaded bytes for every other kind, never from the sender's own
// file name. Either way the name is built from cacheIdentity, not
// m.ID alone, so a kind change on edit never reuses an older cache
// entry.
func (c *Commands) fetchIntoCache(ctx context.Context, m domain.Message, fill func(context.Context, string) error) (string, error) {
	if name, ok := fixedMediaName(m); ok {
		return c.cache.Fetch(ctx, name, fill)
	}

	id := cacheIdentity(m)
	return c.cache.FetchNamed(ctx, id, fill, func(part string) (string, error) {
		contentType, err := sniffContentType(part)
		if err != nil {
			return "", err
		}

		return id + extensionFor(contentType), nil
	})
}

// cacheIdentity names m's slot in the media cache as its id combined
// with its media kind, so an edit that changes the kind never reuses
// an earlier kind's cached file.
func cacheIdentity(m domain.Message) string {
	return m.ID + "." + m.Media.Kind
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
// and true: a fixed extension, never the sender's own filename. A
// video is always cached as ".mp4" even when the bytes turn out to be
// a GIF, since ffmpeg's demuxer sniffs the real format from content,
// not extension. ok is false for every other kind, whose name is only
// known once the bytes exist to sniff.
func fixedMediaName(m domain.Message) (name string, ok bool) {
	switch m.Media.Kind {
	case domain.MediaPhoto:
		return cacheIdentity(m) + ".jpg", true
	case domain.MediaVideo:
		return cacheIdentity(m) + ".mp4", true
	default:
		return "", false
	}
}
