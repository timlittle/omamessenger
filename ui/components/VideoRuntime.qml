import QtQuick
import QtMultimedia
import "../lib/Links.js" as Links

// The actual decode-and-render engine for inline video and GIF playback,
// kept in its own file for the same reason VoiceNoteRuntime is: importing
// the optional QtMultimedia module anywhere else would make the whole UI
// fail to load on a machine without it. InlineVideo loads this through a
// Loader and reads Loader.status to tell whether it is there at all.
// Unlike VoiceNoteRuntime, this has a visible surface (VideoOutput), so
// one instance belongs to each video bubble rather than being shared for
// the whole window: scrolling a video out of view destroys its Loader
// (see InlineVideo), which drops this engine and its decoder with it.
Item {
  id: root

  // source is the local file to play, or "" to stop and release it.
  property string source: ""
  // playing starts or pauses playback of source.
  property bool playing: false
  // muted silences source without stopping it.
  property bool muted: true
  // loop restarts source from the beginning instead of ending, for a
  // video the sending service marked as a silent, looping "GIF".
  property bool loop: false

  // positionMs is how far into source playback has reached.
  readonly property real positionMs: player.position
  // durationMs is source's total length once known, else 0.
  readonly property real durationMs: player.duration > 0 ? player.duration : 0

  // ended fires once playback reaches the end of a non-looping source.
  signal ended()
  // failed fires once the player reports it cannot play source at all,
  // carrying MediaPlayer's own numeric error code, never its
  // errorString: that text can echo the file's own path, which must
  // stay out of logs (see privacy.md).
  signal failed(int code)

  // seek moves playback to ms into source.
  function seek(ms: real): void {
    player.setPosition(ms);
  }

  onSourceChanged: player.source = Links.fileURL(root.source)
  onPlayingChanged: root.playing ? player.play() : player.pause()
  onMutedChanged: audioOutput.muted = root.muted
  onLoopChanged: player.loops = root.loop ? MediaPlayer.Infinite : 1

  MediaPlayer {
    id: player

    videoOutput: output
    audioOutput: AudioOutput { id: audioOutput; muted: root.muted }
    onMediaStatusChanged: if (player.mediaStatus === MediaPlayer.EndOfMedia && !root.loop) root.ended()
    onErrorOccurred: (error, errorString) => root.failed(error)
  }

  VideoOutput {
    id: output
    anchors.fill: parent
  }
}
