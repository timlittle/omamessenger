package app

// play.go builds a message's in-window play copy, the sandboxed
// transcoder's re-encoded version of a video, voice note or audio
// message, the only file the UI ever plays in place (see
// docs/decisions.md). A failed transcode is never an error: FetchMedia
// just reports no playPath, and the UI falls back to opening the
// original externally.

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

// playDurationCapSeconds bounds a message's claimed duration before
// playCopy will even try to transcode it, matching the Video,
// Animation and Audio profiles' own encode cap, so a longer claim
// skips the transcode rather than produce a silently truncated copy
// (see docs/decisions.md).
const playDurationCapSeconds = 600

// playProfileFunc is one of Transcoder's five methods.
type playProfileFunc func(ctx context.Context, in, outDir string) (string, error)

// playCopy prepares m's in-window play copy from original, building it
// once and sharing it with later play fetches via the media cache's
// per-id lock. Returns "" whenever there is nothing to play: a kind
// play doesn't apply to, an over-long duration, a message that already
// failed once this run, or the transcoder failing now.
func (c *Commands) playCopy(ctx context.Context, m domain.Message, original string) string {
	profile, ok := c.playProfile(m)
	if !ok {
		return ""
	}
	if m.Media.Duration > playDurationCapSeconds {
		return ""
	}

	id := playCopyID(m)
	if c.refusedPlay.refused(id) {
		return ""
	}

	path, err := c.transcodedPlayCopy(ctx, m, original, profile)
	if err != nil {
		c.logPlayFailure(m, err)
		c.refusedPlay.remember(id)
		return ""
	}

	return path
}

// playCopyID returns the cache slot m's play copy is kept under: a
// "-play" suffix on cacheIdentity(m), so a lookup for the original
// can never resolve to this derived copy (see docs/decisions.md).
func playCopyID(m domain.Message) string {
	return cacheIdentity(m) + "-play"
}

// playProfile returns the Transcoder method m's kind plays through,
// and true; false for a kind play doesn't apply to, or no Transcoder.
func (c *Commands) playProfile(m domain.Message) (playProfileFunc, bool) {
	if c.transcoder == nil || m.Media == nil {
		return nil, false
	}

	switch m.Media.Kind {
	case domain.MediaVideo:
		if m.Media.Animated {
			return c.transcoder.Animation, true
		}
		return c.transcoder.Video, true
	case domain.MediaVoice, domain.MediaAudio:
		return c.transcoder.Audio, true
	default:
		return nil, false
	}
}

// transcodedPlayCopy returns the path of m's play copy, filling it
// once by running profile against original and moving the result into
// the media cache, under playCopyID with whatever extension the
// transcoder actually produced.
func (c *Commands) transcodedPlayCopy(ctx context.Context, m domain.Message, original string, profile playProfileFunc) (string, error) {
	id := playCopyID(m)
	var ext string

	fill := func(ctx context.Context, dst string) error {
		out, err := profile(ctx, original, c.transcodeJobRoot)
		if err != nil {
			return err
		}
		defer os.RemoveAll(filepath.Dir(out))

		ext = filepath.Ext(out)
		return os.Rename(out, dst)
	}
	nameFor := func(string) (string, error) {
		return id + ext, nil
	}

	return c.cache.FetchNamed(ctx, id, fill, nameFor)
}

// playFailureCategory is the safe word logPlayFailure records for one
// of transcode's sentinel errors, never the error's own text (see
// privacy.md).
func playFailureCategory(err error) string {
	switch {
	case errors.Is(err, transcode.ErrUnavailable):
		return "play-unavailable"
	case errors.Is(err, transcode.ErrTimeout):
		return "play-timeout"
	case errors.Is(err, transcode.ErrTooLong):
		return "play-too-long"
	default:
		return "play-failed"
	}
}

// logPlayFailure records a play copy that could not be built, as the
// safe failure class only, never a path or ffmpeg's own output (see
// privacy.md). Not a *MediaFetchError: the fetch still succeeds
// overall, just without a playPath.
func (c *Commands) logPlayFailure(m domain.Message, err error) {
	category := playFailureCategory(err)
	c.recentErrors.record(category)

	if c.logger == nil {
		return
	}

	kind := ""
	if m.Media != nil {
		kind = m.Media.Kind
	}

	c.logger.Printf("media: play copy failed (kind=%s, reason=%s)", kind, category)
}
