.pragma library

// Media dispatch: which view a message bubble should show for its media,
// robust to messages stored before the "voice" kind existed. The helper
// has no mime type to give the UI, so this leans on a media item's own
// kind first and its file name only as a fallback signal.

// AUDIO_EXTENSIONS are file name endings the in-window voice player can
// play. A voice note received before the helper's "voice" kind existed
// (see docs/decisions.md) is stored forever with kind "file", since a
// message's media is written once and never re-normalized; matching its
// name is the only way left to tell it apart from an ordinary document.
// Routing any audio file this way, not just an old voice note, is a
// deliberate choice: the player can play them all, so there is no
// reason to send the user to an external application for one.
var AUDIO_EXTENSIONS = ['ogg', 'oga', 'opus', 'mp3', 'm4a', 'wav', 'aac', 'amr', 'weba', 'flac'];

// KNOWN_KINDS are the media kinds the helper itself normalizes to and
// names explicitly, each with its own view. A kind in this list is
// never re-guessed from its file name, so a future kind (such as a
// poll) only needs adding here, not touching the fallback logic below.
var KNOWN_KINDS = ['photo', 'video', 'link', 'voice', 'sticker', 'poll'];

// NO_VOICE_NOTE is the voiceNotes default every view that reads
// VoiceNoteController's playback state falls back to before a real one
// is wired in: nothing playing, nothing available. A shared constant
// instead of each view writing out the same object literal.
var NO_VOICE_NOTE = { available: false, playingId: '', positionMs: 0, durationMs: 0 };

// kindFor says which view should render a message's media: one of
// KNOWN_KINDS, "file", or null for no media at all. It never answers
// "photo" or "video" for anything but those exact kinds, so a misnamed
// or legacy audio file can never reach the image view and fail to
// decode there.
function kindFor(media) {
  if (!media || !media.kind) {
    return null;
  }

  if (KNOWN_KINDS.includes(media.kind)) {
    return media.kind;
  }

  return isAudioFileName(media.fileName) ? 'voice' : 'file';
}

// isAudioFileName reports whether name's extension is one the voice
// player can play, used to recognise an audio file the connector only
// ever marked as a plain file.
function isAudioFileName(name) {
  if (!name) {
    return false;
  }

  const dot = name.lastIndexOf('.');
  if (dot < 0) {
    return false;
  }

  return AUDIO_EXTENSIONS.includes(name.slice(dot + 1).toLowerCase());
}
