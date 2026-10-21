.pragma library

// Media dispatch: which view a message bubble should show for its media.
// Every decision here reads only the helper's own normalized media kind,
// never a sender's claimed file name: a sender's name must never choose
// a decoder (see docs/decisions.md). A voice note received before the
// helper's "voice" kind existed is stored forever with kind "file" (see
// docs/decisions.md) and now shows as a plain file row, openable only
// through the gated external-open path, rather than guessing from its
// name that it is really audio.

// KNOWN_KINDS are the media kinds the helper itself normalizes to and
// names explicitly, each with its own view. A kind in this list is
// never re-guessed from its file name, so a future kind (such as a
// poll) only needs adding here, not touching the fallback logic below.
var KNOWN_KINDS = ['photo', 'video', 'link', 'voice', 'sticker', 'poll'];

// NO_VOICE_NOTE is the voiceNotes default every view that reads
// VoiceNoteController's playback state falls back to before a real one
// is wired in: nothing playing, nothing available. A shared constant
// instead of each view writing out the same object literal.
var NO_VOICE_NOTE = { available: false, playingId: '', playing: false, positionMs: 0, durationMs: 0 };

// kindFor says which view should render a message's media: one of
// KNOWN_KINDS, "file", or null for no media at all. It never answers
// "photo", "video" or "voice" for anything but those exact kinds, so a
// misnamed or legacy audio file can never reach the image or in-window
// audio view from its name alone and fail to decode there, or worse,
// be decoded unchecked.
function kindFor(media) {
  if (!media || !media.kind) {
    return null;
  }

  if (KNOWN_KINDS.includes(media.kind)) {
    return media.kind;
  }

  return 'file';
}

// isAnimated reports whether media is a video the sending service itself
// marked as a silent, looping "GIF" (see domain.Media.Animated), so the
// UI autoplays it muted and looping instead of waiting for a click, the
// same way a real animated image would.
function isAnimated(media) {
  return !!media && media.kind === 'video' && !!media.animated;
}

// decodesAsGif reports whether decodeAs - media.fetch's own result
// field (see docs/api.md), never a message's fileName or animated
// flag - says a downloaded video's actual bytes are a genuine,
// sanitized GIF image that must play through an image engine
// (AnimatedImage) rather than a video one: both an ordinary video and
// one a sending service merely marked to play back like a silent,
// looping "GIF" are actually MP4 bytes, which only a video engine can
// decode. The helper is the only thing that ever sets this, from the
// file's own bytes once it is downloaded, which is why nothing here
// reads the media object itself any more.
function decodesAsGif(decodeAs) {
  return decodeAs === 'gif';
}

// canPlayInline reports whether a downloaded video's bytes may be
// loaded into the in-window video player at all: a genuine GIF image
// (decodeAs "gif", see decodesAsGif) always can, through the image
// engine, regardless of playable; an ordinary video can only once
// playable is true, media.fetch's own verdict that the helper has
// validated its container and stream headers (see docs/api.md).
// Never computed from the video's own fileName or animated flag,
// both fully sender-controlled, and never true just because playable
// is true but the bytes turned out to be a GIF image instead - that
// case is always routed through decodesAsGif's own branch, never this
// one, since a GIF image is never a valid video container to begin
// with.
function canPlayInline(decodeAs, playable) {
  return decodesAsGif(decodeAs) || !!playable;
}

// mimeFor names the MIME type to offer when copying media's full file to
// the clipboard, or null for a kind there is nothing sensible to copy as
// (a link preview, a poll, a sticker still only on the service, …). The
// helper always normalizes a photo to a JPEG and a video to MP4 when it
// downloads one (see mediaFileName in media.go), so these are fixed,
// never read from the message itself.
function mimeFor(media) {
  if (!media) {
    return null;
  }

  if (media.kind === 'photo') {
    return 'image/jpeg';
  }

  if (media.kind === 'video') {
    return 'video/mp4';
  }

  return null;
}
