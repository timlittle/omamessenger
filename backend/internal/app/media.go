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
// voice note, together with width and height when it is a photo or
// sticker: its own real dimensions, as displayCopy's own decode of
// it just confirmed them, for the UI to size its decode from in
// place of the message's own claimed size, which need not match what
// the served file actually decodes to (see docs/decisions.md). Both
// are zero for every other media kind. An
// outgoing message's own attachment is already on this machine, so
// its local copy is returned at once, with no remote id needed;
// anything else is downloaded from the service the first time, then
// kept in the media cache. auto is true for a fetch the UI made on
// its own, with no click from the user - today, only an animated
// "GIF" video autoplaying as soon as it is shown - which every
// connector bounds to a far smaller size than a deliberate click is
// trusted with (see connector.MaxBytes); SaveMedia, always a
// deliberate action, passes false. play, when true, also asks for the
// message's in-window play copy (see playCopy): playPath is "" unless
// play was true and the transcode actually succeeded, never an error
// on its own either way, so a transcode failing or being unavailable
// never fails a fetch that would otherwise have succeeded (see
// docs/decisions.md). A download failure is always logged with its
// safe reason category and returned as a *MediaFetchError, so nothing
// fails silently (see docs/decisions.md).
func (c *Commands) FetchMedia(ctx context.Context, messageID string, auto, play bool) (path, playPath string, width, height int, err error) {
	if strings.TrimSpace(messageID) == "" {
		return "", "", 0, 0, fmt.Errorf("%w: messageId is required", ErrInvalidInput)
	}

	m, err := c.store.Message(ctx, messageID)
	if err != nil {
		return "", "", 0, 0, err
	}

	if m.Media == nil || m.Media.Kind == domain.MediaLink {
		return "", "", 0, 0, fmt.Errorf("%w: this message has nothing to download", ErrInvalidInput)
	}

	if reason, ok := c.refused.lookup(cacheIdentity(m)); ok {
		return "", "", 0, 0, &MediaFetchError{Reason: reason, err: errRefusedDisplayCopyCached}
	}

	conv, err := c.store.Conversation(ctx, m.ConversationID)
	if err != nil {
		return "", "", 0, 0, err
	}

	if local, ok := c.localAttachment(m); ok {
		path, width, height, err = c.servedPath(ctx, conv, m, local, false)
	} else {
		path, width, height, err = c.fetchAndServe(ctx, conv, m, auto)
	}
	if err != nil {
		return "", "", 0, 0, err
	}

	if play {
		playPath = c.playCopy(ctx, m, path)
	}

	return path, playPath, width, height, nil
}

// fetchAndServe downloads m's media from its service, for FetchMedia's
// own use once it has found nothing local to serve already, and runs
// servedPath's own guarantee over whatever comes back, the same as
// any other path FetchMedia returns.
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
// must pass before FetchMedia ever hands its file to the UI's own
// Image, and true: a photo's full-size profile, or a sticker's far
// smaller thumbnail one, since a sticker is always drawn small (see
// imagesafe.Thumbnail) - a real GIF image passes through whichever of
// these its own message kind already is, decoded as its own first
// frame like any other still image, with no profile of its own any
// more (see imagesafe.Reencode and docs/decisions.md). A video, voice
// note or audio message has no display-copy profile at all: its own
// in-window play copy, when one exists, comes from playCopy instead,
// never from this function, so ok is false for every one of them.
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

// servedPath is the one guarantee FetchMedia relies on before it ever
// hands a path back: for a photo or sticker, path - the original,
// downloaded or outgoing, bytes - is never that path at all, but
// displayCopy's own display copy built from it, the only file this
// helper ever lets the UI load into a QML Image (see
// docs/decisions.md). refetchable is true once for a download
// recoverUnsafeImage can retry from the service when building that
// copy fails; false for an outgoing message's own copy, which has
// nowhere else to come from. Every other media kind passes path
// straight through unchecked here: it already has its own per-fetch
// check elsewhere.
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

// tryDisplayCopy builds m's own display copy from originalPath (see
// displayCopy), removing originalPath itself only when the failure
// was imagesafe refusing its own bytes: a file that cannot be turned
// into something safe to show is never left trusted on disk just
// because of how or when it got there, whether this is servedPath's
// own first attempt or recoverUnsafeImage's retry (see
// docs/decisions.md). Any other failure - a cache write hitting a
// full disk, say - says nothing about originalPath's own bytes, so it
// is left exactly as it was: for an outgoing message, that is the
// attachment a still-pending send needs to upload, not a file this
// helper owns to delete on a whim.
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

// displayCopyID names m's own slot for its display copy in the
// downloaded media cache: distinct from cacheIdentity's own slot for
// the original bytes it is built from, and a name no release before
// this one ever wrote, so a cache hit under it is always this
// release's own Reencode output, never a file an older release's
// different checks already passed - the one invariant that makes
// re-checking a cache hit unnecessary at all (see
// docs/decisions.md). The same id serves a downloaded photo and an
// outgoing one's own bubble alike, since both resolve to the same
// m.ID and m.Media.Kind regardless of where their original bytes
// currently live. The "-view" suffix, not ".view", matters: findByID
// matches a stored file's id against everything up to its own next
// '.', so cacheIdentity(m) alone - the original's own slot for a kind
// fetched through FetchNamed, such as a voice note or sticker - must
// never be a prefix of this id followed by '.', or a lookup for the
// original could resolve to this derived copy instead (see
// cache.findByID and docs/decisions.md).
func displayCopyID(m domain.Message) string {
	return cacheIdentity(m) + "-view"
}

// displayCopy returns the path of m's own display copy, filling it -
// once - by re-encoding originalPath's bytes through
// connector.ReencodeImageFile under profile when no copy exists yet.
// Re-encoding is a lossy, one-way rewrite, so it must never run again
// once a copy exists: unlike the per-serve check this replaced,
// FetchMedia can be called many times for the same photo as its
// message bubble scrolls in and out of view, and a cache hit here is
// served as is, with no re-check (see docs/decisions.md). width and
// height are read back from the result's own header on every call,
// including a cache hit (see imagesafe.Dimensions): cheap enough to
// pay every time, unlike Reencode's own full decode.
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

// reencodeForDisplay is displayCopy's own fill: the ordinary
// connector.ReencodeImageFile re-encode, falling back to the
// sandboxed transcoder's StillFrame profile, then that single
// extracted frame's own Reencode, only when the ordinary re-encode
// refuses the bytes outright and a transcoder is actually wired in.
// This is what makes a sticker format Go's own decoders cannot read
// at all - an animated WebP, at least - still get a display copy:
// StillFrame's ffmpeg decodes it where imagesafe never could, and the
// frame it hands back then passes through exactly the same check and
// encoder every other display copy does (see docs/decisions.md). A
// failure that is not ErrUnsafeImage - originalPath too large or
// unreadable - is returned as is, never worth a sandboxed job over.
// Each of the (up to two) calls into imagesafe's own full decode
// acquires this helper's own decode slot on its own (see
// reencodeImageFile and decodeSlots), released again before the
// sandboxed StillFrame job runs: that job has no use for the slot
// imagesafe's own decode needs, and holding it idle for however long
// ffmpeg takes would only ever delay some other request's own photo
// for no reason.
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

// dimensionsOf reads width and height from path's own header through
// imagesafe.Dimensions, never a full decode: the cheap check
// displayCopy's own cache hit gets on every serve.
func dimensionsOf(path string) (width, height int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close() // reading only; nothing to lose on a close error

	return imagesafe.Dimensions(f)
}

// recoverUnsafeImage responds to servedPath's own display copy
// failing to build, which tryDisplayCopy has already removed the
// original it tried to build from: a download gets exactly one
// fresh retry from the service, since tryDisplayCopy's own removal
// guarantees this retry's download actually asks the service for
// something rather than finding the same bad file cached under the
// same name again (see cache.Cache.Fetch). Anything else - an
// outgoing message's own local copy, with nowhere further to come
// from, or a retry that fails the same check again - is classified
// and logged the same way any other failed fetch is (see
// failDecodeCheck), and never served. profile is recomputed from m's
// own kind rather than passed in, since servedPath already confirmed
// imageSafeProfile finds one for this message before ever calling
// here.
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

// failDecodeCheck classifies and logs a failure from recoverUnsafeImage's
// own retry - a photo or sticker that still fails its display-copy
// check on the one re-download it gets - the same way download
// classifies and logs a failed fetch, so media that turned out to be
// unreadable once its actual bytes were checked is never silently
// left as it was. It also remembers m under c.refused, so a later
// FetchMedia for the same message fails fast with this same reason
// rather than downloading and decoding it all over again (see
// refusedDisplayCopies): the UI auto-fetches every visible photo, so
// without this, a refused photo or sticker would pay this exact cost
// again on every scroll.
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

// fetchIntoCache fills the media cache for m: Fetch under a fixed, safe
// name for a photo or video (see fixedMediaName), never chosen from
// anything the sender claims, or FetchNamed's own sniffed one for
// every other kind, chosen only once fill's downloaded bytes exist, so
// a file, voice note or sticker's cache name is never again picked
// from the sender's own file name the way it used to be (see
// docs/decisions.md). Either way the name is built from cacheIdentity,
// never m.ID alone, so an edit that changes m's own media kind is
// never served whatever an earlier kind already had cached under the
// same id.
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

// cacheIdentity names m's own slot in the downloaded media cache: its
// id combined with its own media kind, so an edit that turns one kind
// of message into another - a voice note replaced by a sticker, say -
// never reuses whatever an earlier kind already had cached under the
// same id (see docs/decisions.md). Every cache name this package
// builds for m, fixed or sniffed, is derived from this same pair,
// never the id alone.
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
// and true: an extension that says what it is, never the sender's
// own filename or attributes, which fetchIntoCache must trust even
// less for these two kinds than for any other, since this is the one
// file playCopy or displayCopy ever reads as that kind's own original.
// Every MediaVideo message is cached as ".mp4", even the rare one
// whose downloaded bytes turn out to be a genuine GIF file rather
// than an actual MP4 container: ffmpeg's own demuxer sniffs a file's
// real format from its content, never its extension, the same as
// Qt's image plugins already do for a photo, so playCopy's re-encode
// produces a real MP4 play copy from either one with nothing extra to
// check here first (see docs/decisions.md). ok is false for every
// other kind, whose correct name can only be known once its own bytes
// exist to sniff (see fetchIntoCache).
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
