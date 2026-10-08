import QtQuick

// Plays voice notes, one at a time, for the whole window. This lives here
// rather than in the message bubble itself because a ListView recycles or
// destroys delegates that scroll out of view; keeping the one MediaPlayer
// and its playback state at this level means scrolling never interrupts
// whatever is playing, and starting a second note always pauses the
// first, since there is only ever one of either here to hold.
//
// The actual decoder (VoiceNoteRuntime.qml) is loaded through a Loader
// because it imports the optional QtMultimedia module: `available` turns
// false if that module is missing, and every method below is then a
// no-op, so a caller can always try them and fall back to opening the
// file externally when nothing happened.
Item {
  id: root

  // runtimeSource is the file the player engine loads: VoiceNoteRuntime
  // by default, overridable so a test can point it at a broken or
  // missing file instead, to drive the same Loader.Error path a machine
  // without QtMultimedia would hit, without needing QtMultimedia to
  // actually be missing from the machine running the test.
  property url runtimeSource: Qt.resolvedUrl("../components/VoiceNoteRuntime.qml")

  // available is false once QtMultimedia itself could not be loaded, so
  // callers know to fall back to opening a voice note externally instead.
  readonly property bool available: _runtime.status !== Loader.Error

  // _engine holds the loaded runtime as a plain var, never Loader's own
  // typed `item`: qmllint only knows that as a bare QtObject, which has
  // none of VoiceNoteRuntime's own properties, so it would flag every one
  // of them as missing. A var has no declared type for qmllint to check
  // against, the same reason `property var service` elsewhere in this UI
  // calls its own methods without a warning.
  property var _engine: _runtime.item

  // playingId is the message id loaded into the player right now, or ""
  // when nothing is: it stays set while paused, so toggle() resumes
  // rather than restarts it.
  property string playingId: ""
  // playing is true while playingId is actually making sound.
  property bool playing: false
  // positionMs/durationMs mirror the runtime's own playback position and
  // the file's length once known, for playingId only.
  property real positionMs: 0
  property real durationMs: 0

  // ended reports the id of a note that just finished playing on its own.
  signal ended(string id)

  // toggle starts id playing from path, pausing whichever other note was
  // playing first, or pauses/resumes id itself when it is already the
  // loaded note. durationHintMs seeds durationMs until the file's own
  // duration is known, so the elapsed/total label has something to show
  // immediately rather than starting blank.
  function toggle(id: string, path: string, durationHintMs: real): void {
    if (!root.available || !path) return;

    if (root.playingId === id) {
      root.playing = !root.playing;
      return;
    }

    root.playingId = id;
    root.durationMs = durationHintMs;
    root.positionMs = 0;
    root._engine.source = path;
    root.playing = true;
  }

  Loader {
    id: _runtime
    source: root.runtimeSource

    onLoaded: root._engine.playing = root.playing
  }

  Connections {
    target: root.available ? root._engine : null

    function onPositionMsChanged() { root.positionMs = root._engine.positionMs; }
    function onDurationMsChanged() { if (root._engine.durationMs > 0) root.durationMs = root._engine.durationMs; }
    function onEnded() {
      root.playing = false;
      root.ended(root.playingId);
    }
    // A file the engine cannot play at all (a legacy cache entry in the
    // wrong format, or a codec this machine's Qt build lacks) must not
    // leave playing stuck true forever with nothing able to flip the
    // button back: this clears the loaded note entirely, so the bubble
    // returns to showing its play glyph and a retry starts fresh.
    function onFailed(code) {
      console.warn("voice note playback failed: code=" + code);
      root.playingId = "";
      root.playing = false;
      root.positionMs = 0;
      root.durationMs = 0;
    }
  }

  onPlayingChanged: if (root.available) root._engine.playing = root.playing
}
