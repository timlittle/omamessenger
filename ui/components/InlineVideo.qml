import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format
import "../lib/ImageBounds.js" as ImageBounds
import "../lib/Media.js" as Media

// A video or "GIF video" in a message, once downloaded. A click
// downloads the file if needed, asks the helper to prepare an
// in-window play copy, and plays it in place: looping and muted for a
// "GIF", with play/pause, seek and mute for an ordinary video. Only
// one video decodes at a time across the chat (see VideoController),
// and it never decodes a sender's own bytes directly - only the
// helper's transcoded playPath (see docs/decisions.md).
Item {
  id: root

  // video is the message's video media: kind, width, height, duration,
  // thumb, animated and fileName.
  required property var video
  // path is where the full file was downloaded, or "" until then.
  property string path: ""
  // playPath is the in-window play copy a play fetch produced, or ""
  // before one has answered; the only file this component ever decodes.
  property string playPath: ""
  // failed is true once a fetch for this video has definitely failed.
  property bool failed: false
  // preparing is true while a play fetch for this video is in flight.
  property bool preparing: false
  // available is false when in-window playback cannot work at all.
  property bool available: true
  // playing is true while this is the one video actually decoding right now.
  property bool playing: false
  // muted mutes a playing, non-animated video; a GIF is always muted.
  property bool muted: true
  // maxWidth is the widest this may be drawn.
  property real maxWidth: Style.space(320)
  // windowActive is whether the user can see the window right now;
  // playback pauses while false.
  property bool windowActive: true
  // runtimeSource is the file the player engine loads; overridable so
  // tests can simulate a machine without QtMultimedia.
  property url runtimeSource: Qt.resolvedUrl("VideoRuntime.qml")
  // expandable shows a click target to open this video or GIF full size.
  property bool expandable: true

  // toggled asks the caller to select, deselect or pause this video.
  signal toggled()
  // muteToggled asks the caller to flip the shared mute preference.
  signal muteToggled()
  // opened asks for this video to be opened externally instead.
  signal opened()
  // expandRequested asks the caller to show this video or GIF full size.
  signal expandRequested()

  // _animated is true for a video the service marked as a silent, looping "GIF".
  readonly property bool _animated: Media.isAnimated(root.video)
  // _verifiedWidth and _verifiedHeight are the thumbnail's own real,
  // decoder-confirmed size, never the sender's claimed width/height.
  readonly property int _verifiedWidth: ImageBounds.verifiedDimensions(root.video, false).width
  readonly property int _verifiedHeight: ImageBounds.verifiedDimensions(root.video, false).height
  // _aspect is height over width, square when the size is unknown.
  readonly property real _aspect: root.video.width > 0 ? root.video.height / root.video.width : 1
  // _decoding is true while this bubble should actually hold the
  // QtMultimedia engine: selected, ready, visible and the window active.
  readonly property bool _decoding: root.available && root.playPath !== "" && root.visible && root.windowActive && root.playing
  // _paused is a selected, non-animated video's local pause state; the
  // engine stays loaded with its last frame on screen.
  property bool _paused: false
  // _engine is the loaded VideoRuntime, as a plain var.
  property var _engine: _runtime.item
  // _unavailable is true once a fetch has failed with no thumbnail to fall back to.
  readonly property bool _unavailable: !root.video.thumb && root.failed && root.path === ""
  // _expandable is true once this video is selected and playing, so a
  // click opens the full-size viewer instead of toggling again.
  readonly property bool _expandable: root.expandable && root.playing && root.available

  implicitWidth: Math.min(root.maxWidth, Style.space(320))
  implicitHeight: Math.min(root.implicitWidth * root._aspect, Style.space(400))

  Loader {
    id: _runtime
    objectName: "videoRuntimeLoader"
    anchors.fill: parent
    active: root._decoding
    source: root.runtimeSource

    onLoaded: {
      root._paused = false;
      _runtime.item.source = root.playPath;
      _runtime.item.loop = root._animated;
      _runtime.item.muted = root._animated || root.muted;
      _runtime.item.playing = true;
    }
  }

  Connections {
    target: root._decoding ? root._engine : null

    function onEnded() { root.toggled(); }
    function onFailed(code) {
      console.warn("inline video playback failed: code=" + code);
      root.toggled();
    }
  }

  // The static thumbnail, shown whenever the engine is not actively
  // decoding: before a click, while paused, or while preparing.
  Image {
    id: thumb
    objectName: "videoThumb"
    anchors.fill: parent
    visible: !root._decoding
    source: root.video.thumb ? "data:" + Media.thumbMime(root.video.thumb) + ";base64," + root.video.thumb : ""
    fillMode: Image.PreserveAspectFit
    // Bound by the thumbnail's verified size, never the sender's claimed one.
    sourceSize.width: ImageBounds.boundedSourceSize(root.implicitWidth, root.implicitHeight, root._verifiedWidth, root._verifiedHeight).width
    sourceSize.height: ImageBounds.boundedSourceSize(root.implicitWidth, root.implicitHeight, root._verifiedWidth, root._verifiedHeight).height
    asynchronous: true
    smooth: true
  }

  Rectangle {
    anchors.fill: parent
    visible: thumb.visible && !root.video.thumb && root.path === ""
    color: Util.alpha(Theme.foreground, 0.08)
  }

  Text {
    objectName: "unavailableLabel"
    anchors.centerIn: parent
    width: parent.width - Theme.spacing.sm * 2
    visible: root._unavailable
    horizontalAlignment: Text.AlignHCenter
    wrapMode: Text.Wrap
    text: root.video.fileName ? "Video unavailable · " + root.video.fileName : "Video unavailable"
    // fileName is sender-controlled: PlainText.
    textFormat: Text.PlainText
    color: Util.alpha(Theme.foreground, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  Text {
    objectName: "gifBadge"
    visible: root._animated
    anchors { left: parent.left; top: parent.top; margins: Theme.spacing.xs }
    text: "GIF"
    textFormat: Text.PlainText
    color: Theme.foreground
    style: Text.Outline
    styleColor: Util.alpha(Theme.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption; weight: Font.DemiBold }
  }

  Text {
    objectName: "playMark"
    anchors.centerIn: parent
    visible: !root._decoding && !root._unavailable && !root.preparing
    text: "▶"
    textFormat: Text.PlainText
    color: Theme.foreground
    style: Text.Outline
    styleColor: Util.alpha(Theme.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.heading }
  }

  Text {
    objectName: "preparingLabel"
    anchors.centerIn: parent
    visible: !root._decoding && !root._unavailable && root.preparing
    text: "Preparing…"
    textFormat: Text.PlainText
    color: Theme.foreground
    style: Text.Outline
    styleColor: Util.alpha(Theme.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  Text {
    anchors { right: parent.right; bottom: parent.bottom; margins: Theme.spacing.xs }
    visible: !root._animated && !root._decoding && text !== ""
    text: Format.duration(root.video.duration)
    textFormat: Text.PlainText
    color: Theme.foreground
    style: Text.Outline
    styleColor: Util.alpha(Theme.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  // Asks to open the full-size viewer once this video is selected and playing.
  MouseArea {
    id: expandArea
    objectName: "videoExpandArea"
    anchors { left: parent.left; right: parent.right; top: parent.top }
    height: controls.visible ? Math.max(0, parent.height - controls.height - Theme.spacing.xs) : parent.height
    visible: root._expandable
    enabled: root._expandable
    cursorShape: Qt.PointingHandCursor
    onClicked: root.expandRequested()
  }

  // The playback controls, shown only while a non-animated video is
  // actually decoding.
  RowLayout {
    id: controls
    objectName: "videoControls"
    visible: !root._animated && root._decoding
    anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: Theme.spacing.xs }
    spacing: Theme.spacing.xs

    Item {
      Layout.preferredWidth: pauseGlyph.implicitWidth
      Layout.preferredHeight: pauseGlyph.implicitHeight

      Text {
        id: pauseGlyph
        objectName: "videoPauseButton"
        text: root._paused ? "▶" : "⏸"
        textFormat: Text.PlainText
        color: Theme.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root._togglePause()
      }
    }

    Item {
      id: seekTrack
      objectName: "videoSeekTrack"
      Layout.fillWidth: true
      implicitHeight: Style.space(4)

      Rectangle {
        anchors.fill: parent
        radius: height / 2
        color: Util.alpha(Theme.background, 0.5)
      }

      Rectangle {
        objectName: "videoSeekFill"
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
        objectName: "videoMuteButton"
        text: root.muted ? "🔇" : "🔊"
        textFormat: Text.PlainText
        color: Theme.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root.muteToggled()
      }
    }
  }

  // _togglePause starts or stops a selected video's playback without deselecting it.
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

  // Starts, selects or opens this video on the first click, before
  // _expandable claims the bubble instead.
  MouseArea {
    anchors.fill: parent
    visible: !controls.visible && !root._expandable
    enabled: root.visible
    cursorShape: Qt.PointingHandCursor
    onClicked: root.available ? root.toggled() : root.opened()
  }
}
