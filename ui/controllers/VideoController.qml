import QtQuick

// Tracks which single non-animated video is selected to play right now,
// pausing any other one the moment a new one starts, and the shared
// mute preference every video bubble reads. An animated GIF needs
// neither: it plays whenever it is visible and the window is active,
// independent of every other one and always muted, since it carries no
// audio in the first place (see InlineVideo).
//
// available says whether in-window video playback can work at all on
// this machine, found once through the same optional-QtMultimedia
// Loader probe VoiceNoteController already uses: importing QtMultimedia
// anywhere outside VideoRuntime.qml would make the whole UI fail to
// load on a machine without it.
Item {
  id: root

  // runtimeSource is the file the probe loads: VideoRuntime by default,
  // overridable so a test can point it at a broken file instead, to
  // drive the Loader.Error path a machine without QtMultimedia would
  // hit, without needing QtMultimedia to actually be missing here.
  property url runtimeSource: Qt.resolvedUrl("../components/VideoRuntime.qml")

  // available is false once the probe reports QtMultimedia could not be
  // loaded, so callers know every video falls back to opening
  // externally instead of playing in place.
  readonly property bool available: _probe.status !== Loader.Error

  // playingId is the selected video's message id, or "" when none is.
  property string playingId: ""
  // muted is the shared mute preference every selected video starts
  // with; an animated GIF always plays muted regardless of this.
  property bool muted: true

  // toggle selects id, pausing whichever other video was selected
  // first, or deselects id when it is already the selected one.
  function toggle(id: string): void {
    root.playingId = root.playingId === id ? "" : id;
  }

  // setMuted changes the shared mute preference.
  function setMuted(value: bool): void {
    root.muted = value;
  }

  // _probe exists only so its Loader.status answers available above,
  // exactly the way VoiceNoteController's own runtime Loader does; it
  // is never shown or asked to play anything.
  Loader {
    id: _probe
    source: root.runtimeSource
    visible: false
  }
}
