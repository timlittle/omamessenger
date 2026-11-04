import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// The full-window video player shown inside the in-app media viewer, with
// play/pause, a seek bar and mute. It only ever plays playPath, the file
// the helper's sandboxed transcoder produced (see docs/decisions.md).
Item {
  id: root

  // video is the open message's video media: kind, fileName, animated,
  // width and height.
  required property var video
  // path is where the full file was downloaded, or "" until then:
  // read only to tell "nothing fetched yet" apart from "fetched but
  // no play copy" in the unavailable label below.
  property string path: ""
  // playPath is the in-window play copy a play fetch produced (see
  // InlineVideo.playPath), or "" before one has answered or succeeded.
  property string playPath: ""
  // available is false when in-window video playback cannot work at
  // all on this machine.
  property bool available: true
  // muted mutes a playing video, shared with every other video in
  // the window, the same preference InlineVideo reads.
  property bool muted: true
  // windowActive is whether the user can see the window at all: shown
  // and focused. Playback stops decoding while it is false, the same
  // as an inline video bubble already does.
  property bool windowActive: true
  // runtimeSource is the file the player engine loads: VideoRuntime by
  // default, overridable so a test can point it at a broken file
  // instead, to drive the Loader.Error path without QtMultimedia
  // actually being missing from the machine running the test.
  property url runtimeSource: Qt.resolvedUrl("VideoRuntime.qml")

  // muteToggled asks the caller to flip the shared mute preference.
  signal muteToggled()

  // _paused is the player's own local pause state.
  property bool _paused: false
  // _engine is the loaded VideoRuntime, as a plain var: see
  // VoiceNoteController for why this is never Loader's own typed item.
  property var _engine: _runtime.item

  // Clicks on the player itself stay on the player: declared first, so
  // it sits behind every other child here and only ever catches a click
  // none of them did, the same way PhotoView's own swallow area, sized
  // to the painted image, keeps a click on the photo from reaching the
  // viewer's backdrop behind it.
  MouseArea {
    objectName: "fullVideoBackgroundArea"
    anchors.fill: parent
  }

  Loader {
    id: _runtime
    objectName: "fullVideoRuntimeLoader"
    anchors.fill: parent
    active: root.available && root.playPath !== "" && root.windowActive
    source: root.runtimeSource

    onLoaded: {
      root._paused = false;
      _runtime.item.source = root.playPath;
      _runtime.item.loop = false;
      _runtime.item.muted = root.muted;
      _runtime.item.playing = true;
    }
  }

  Text {
    objectName: "fullVideoUnavailableLabel"
    anchors.centerIn: parent
    width: parent.width - Theme.spacing.sm * 2
    // Shown whether QtMultimedia is missing or the helper produced no play copy.
    visible: root.path !== "" && (!root.available || root.playPath === "")
    horizontalAlignment: Text.AlignHCenter
    wrapMode: Text.Wrap
    text: "This video can't be played here."
    textFormat: Text.PlainText
    color: Util.alpha(Theme.foreground, 0.7)
    font { family: Theme.font.family; pixelSize: Theme.font.body }
  }

  RowLayout {
    id: controls
    objectName: "fullVideoControls"
    visible: root.available && root.playPath !== ""
    anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Theme.spacing.md }
    spacing: Theme.spacing.sm

    Item {
      Layout.preferredWidth: pauseGlyph.implicitWidth
      Layout.preferredHeight: pauseGlyph.implicitHeight

      Text {
        id: pauseGlyph
        objectName: "fullVideoPauseButton"
        text: root._paused ? "▶" : "⏸"
        textFormat: Text.PlainText
        color: Theme.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.heading }
      }

      MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root._togglePause()
      }
    }

    Item {
      id: seekTrack
      objectName: "fullVideoSeekTrack"
      Layout.fillWidth: true
      implicitHeight: Style.space(6)

      Rectangle {
        anchors.fill: parent
        radius: height / 2
        color: Util.alpha(Theme.background, 0.5)
      }

      Rectangle {
        objectName: "fullVideoSeekFill"
        anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
        radius: height / 2
        color: Theme.accent
        width: {
          const duration = root._engine ? root._engine.durationMs : 0;
          const position = root._engine ? root._engine.positionMs : 0;
          return duration > 0 ? seekTrack.width * Math.min(1, position / duration) : 0;
        }
      }

      MouseArea {
        anchors.fill: parent
        onPressed: (mouse) => root._seekTo(mouse.x)
        onPositionChanged: (mouse) => { if (pressed) root._seekTo(mouse.x); }
      }
    }

    Item {
      Layout.preferredWidth: muteGlyph.implicitWidth
      Layout.preferredHeight: muteGlyph.implicitHeight

      Text {
        id: muteGlyph
        objectName: "fullVideoMuteButton"
        text: root.muted ? "🔇" : "🔊"
        textFormat: Text.PlainText
        color: Theme.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.heading }
      }

      MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root.muteToggled()
      }
    }
  }

  // _togglePause starts or stops playback without tearing the engine
  // down, so its last frame stays on screen either way.
  function _togglePause(): void {
    root._paused = !root._paused;
    if (root._engine) root._engine.playing = !root._paused;
  }

  // _seekTo moves playback to the position x, a point along the seek
  // track, corresponds to.
  function _seekTo(x: real): void {
    if (!root._engine || root._engine.durationMs <= 0) return;

    const fraction = Math.max(0, Math.min(1, x / seekTrack.width));
    root._engine.seek(fraction * root._engine.durationMs);
  }
}
