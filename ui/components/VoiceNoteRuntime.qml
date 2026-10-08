import QtQuick
import QtMultimedia

// The actual decode-and-play engine for voice notes, kept in its own file
// because QtMultimedia is an optional system dependency (not every Omarchy
// install has it): VoiceNoteController loads this file through a Loader
// and reads Loader.status to tell whether it is there at all. Importing
// QtMultimedia anywhere else would make the whole UI fail to load on a
// machine without it, instead of just losing in-window voice playback.
Item {
  id: root

  // source is the local file to play, or "" to stop and release it.
  property string source: ""
  // playing starts or pauses playback of source.
  property bool playing: false
  // rate is the playback speed multiplier.
  property real rate: 1.0

  // positionMs is how far into source playback has reached.
  readonly property real positionMs: player.position
  // durationMs is source's total length once known, else 0.
  readonly property real durationMs: player.duration > 0 ? player.duration : 0

  // ended fires once playback reaches the end of source.
  signal ended()
  // failed fires once the player reports it cannot play source at all,
  // carrying MediaPlayer's own numeric error code, never its
  // errorString: that text can echo the file's own path, which must
  // stay out of logs (see privacy.md).
  signal failed(int code)

  onSourceChanged: player.source = root.source ? "file://" + root.source : ""
  onPlayingChanged: root.playing ? player.play() : player.pause()
  onRateChanged: player.playbackRate = root.rate

  MediaPlayer {
    id: player

    audioOutput: AudioOutput {}
    onMediaStatusChanged: if (player.mediaStatus === MediaPlayer.EndOfMedia) root.ended()
    onErrorOccurred: (error, errorString) => root.failed(error)
  }
}
