package app

// play.go builds a message's in-window play copy: the file the
// sandboxed transcoder re-encoded a video, "GIF video", voice note or
// audio message's own original into, the only file the UI ever plays
// in place for those kinds (see docs/decisions.md). Never an error of
// its own - a transcode failing, timing out or simply being
// unavailable just means FetchMedia reports no playPath, so the UI
// falls back to opening the original externally.

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

// playDurationCapSeconds bounds a message's own claimed duration
// before playCopy will even try to transcode it: transcode's own
// Video, Animation and Audio profiles each cap how much of their
// input they actually encode at this same ten minutes (see
// docs/decisions.md). A message claiming longer than that would only
// ever produce a silently truncated play copy, so playCopy skips the
// transcode entirely and leaves the UI to open the file externally
// instead, rather than play a copy that quietly cuts off partway
// through. This package cannot import transcode's own unexported
// duration constants (see .golangci.yml's depguard rules), so the
// figure is duplicated here, the same way transcode's own
// videoDurationSeconds duplicates videosafe's former limit.
const playDurationCapSeconds = 600

// playProfileFunc is one of Transcoder's five methods.
type playProfileFunc func(ctx context.Context, in, outDir string) (string, error)

// playCopy prepares m's own in-window play copy, built from
// original - m's downloaded bytes or, for an outgoing message, its
// own local attachment, never a photo or sticker's display copy,
// which play never applies to - the first time anything asks to play
// it, and shared by every later play fetch of the same message
// through the media cache's own per-id lock (see
// transcodedPlayCopy). It reports "" whenever there is nothing to
// play: a kind play does not apply to, a claimed duration over the
// chosen profile's own cap, or the transcoder itself failing or being
// unavailable - never an error, since the UI opens the file
// externally either way (see docs/decisions.md).
func (c *Commands) playCopy(ctx context.Context, m domain.Message, original string) string {
	profile, ok := c.playProfile(m)
	if !ok {
		return ""
	}
	if m.Media.Duration > playDurationCapSeconds {
		return ""
	}

	path, err := c.transcodedPlayCopy(ctx, m, original, profile)
	if err != nil {
		c.logPlayFailure(m, err)
		return ""
	}

	return path
}

// playProfile returns the Transcoder method m's own kind plays
// through, and true: Animation for a video a sending service marked
// to autoplay silently (see domain.Media.Animated), Video for any
// other video, Audio for a voice note or an ordinary audio message.
// false for every other kind, which play never applies to, or when no
// Transcoder was wired in at all.
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

// transcodedPlayCopy returns the path of m's own play copy, filling
// it - once - by running profile against original into a fresh job
// directory under c.transcodeJobRoot and moving the result into the
// media cache, then removing that job directory, under a name built
// from m's own cache identity but with "-play" and the extension the
// transcoder actually produced appended - never assumed up front,
// since the profile it runs is free to change its own output
// container. The "-play" separator, a '-' and not a '.', matters the
// same way displayCopyID's own "-view" does: cache.findByID must
// never be able to resolve a lookup for the original's own id
// (cacheIdentity(m) alone) to this derived copy instead (see
// docs/decisions.md).
func (c *Commands) transcodedPlayCopy(ctx context.Context, m domain.Message, original string, profile playProfileFunc) (string, error) {
	id := cacheIdentity(m) + "-play"
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
// of transcode's own sentinel errors, never the error's own text
// (see privacy.md).
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

// logPlayFailure records a play copy that could not be built: the
// safe failure class only, for Doctor's own recent-errors check and
// the helper's log, never a path, a name or ffmpeg's own stderr (see
// privacy.md). This is deliberately not routed through
// *MediaFetchError: a play fetch that fails this way still succeeds
// overall, just without a playPath, so there is no failed request for
// the UI to show a reason for.
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
