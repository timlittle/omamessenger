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
// "audio" is an ordinary audio message (not push-to-talk): it shares
// "voice"'s inline player, gated the same way by media.fetch's own
// playable verdict, but with its own file name and duration shown
// instead of a voice note's fixed label (see VoiceNotePlayer.qml).
var KNOWN_KINDS = ['photo', 'video', 'link', 'voice', 'audio', 'sticker', 'poll'];

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

// thumbMime names the MIME type thumb's own base64 bytes actually
// decode as, sniffed from its own leading characters rather than
// assumed from the message's kind: the helper's own imagesafe.Reencode
// can write a thumbnail as either PNG or JPEG depending on whether it
// carries real transparency - a sticker's cut-out shape, say - whatever
// kind of message it illustrates (see docs/decisions.md), so a data
// URI built from it must declare whichever one it actually is.
// "iVBORw0KGgo" is the base64 encoding every PNG's own fixed signature
// bytes always start with; any other thumb - including one with no
// bytes at all - defaults to JPEG, the more common of the two.
function thumbMime(thumb) {
  if (thumb && thumb.indexOf('iVBORw0KGgo') === 0) {
    return 'image/png';
  }

  return 'image/jpeg';
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

// copyable reports whether media's full file may sensibly be offered
// for copying to the clipboard: mimeFor already limits this to a
// photo or a video, which is as far as the UI needs to decide -
// whether a video's own play copy can actually be built is the
// helper's own call, made fresh inside media.copy itself (see
// docs/decisions.md), not something a cached, possibly stale local
// verdict should gate here.
function copyable(media) {
  return !!mimeFor(media);
}
