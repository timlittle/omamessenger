import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format
import "../lib/Media.js" as Media

// A video or GIF in a message, once downloaded. A video the sending
// service marked as a silent, looping "GIF" (see domain.Media.Animated)
// autoplays muted and looping as soon as it is here, the same way a
// photo asks for its full image as soon as it is shown; an ordinary
// video stays a thumbnail with a play mark and its length until clicked,
// then plays in place with play/pause, a draggable seek bar and a mute
// button. Either kind stops decoding - the engine is torn down, not
// merely paused - once this bubble scrolls out of view or the window
// loses focus, so a long scroll-back through a chat full of GIFs never
// keeps every one of them decoding at once.
//
// available is false when QtMultimedia itself could not be loaded (see
// VideoController): a real .gif file still animates through
// AnimatedImage, which needs no such module, but an animated "GIF" that
// is actually a silent MP4 - true for every WhatsApp or Telegram one -
// can only fall back to a still thumbnail, and an ordinary video falls
// back to opening externally, exactly as it did before this component
// existed.
Item {
  id: root

  // video is the message's video media: kind, width, height, duration,
  // thumb, animated and fileName.
  required property var video
  // path is where the full file was downloaded, or "" until then.
  property string path: ""
  // failed is true once a fetch for this video has definitely failed.
  property bool failed: false
  // available is false when in-window playback (for anything but a real
  // .gif file) cannot work at all: see VideoController.available.
  property bool available: true
  // playing is true while this is the one video (of the single slot
  // every non-animated video shares) actually decoding right now.
  // Ignored for an animated GIF, which plays whenever it is visible and
  // the window is active, independent of every other one.
  property bool playing: false
  // muted mutes a playing, non-animated video; an animated GIF is
  // always muted regardless of this, since it carries no audio worth
  // unmuting in the first place.
  property bool muted: true
  // maxWidth is the widest this may be drawn.
  property real maxWidth: Style.space(320)
  // windowActive is whether the user can see the window at all: shown
  // and focused. Playback pauses while it is false, the same as it does
  // while this bubble has scrolled out of view.
  property bool windowActive: true
  // runtimeSource is the file the player engine loads: VideoRuntime by
  // default, overridable so a test can point it at a broken or missing
  // file instead, to drive the Loader.Error path a machine without
  // QtMultimedia would hit, without needing QtMultimedia to actually be
  // missing from the machine running the test.
  property url runtimeSource: Qt.resolvedUrl("VideoRuntime.qml")

  // wanted asks for an animated GIF's file to be downloaded; never
  // fired for an ordinary video, which downloads only once asked for.
  signal wanted()
  // toggled asks the caller to start or pause this video; only
  // meaningful while available is true.
  signal toggled()
  // muteToggled asks the caller to flip the shared mute preference.
  signal muteToggled()
  // opened asks for this video to be opened externally instead: the
  // fallback a click takes for an ordinary video while available is
  // false, exactly what it did before in-window playback existed.
  signal opened()

  // _animated is true for a video the service marked as a silent,
  // looping "GIF".
  readonly property bool _animated: Media.isAnimated(root.video)
  // _realGifFile is true when the file itself is an actual .gif, the
  // one animated format AnimatedImage can decode without QtMultimedia;
  // every GIF WhatsApp or Telegram actually sends is a silent MP4, so
  // this is rare, but a real one still deserves to animate.
  readonly property bool _realGifFile: /\.gif$/i.test(root.video.fileName || "")
  // _aspect is height over width, square when the size is unknown.
  readonly property real _aspect: root.video.width > 0 ? root.video.height / root.video.width : 1
  // _decoding is true while this bubble should actually hold the engine:
  // an animated GIF whenever it is on screen and the window is active,
  // or the one non-animated video the caller selected (playing), under
  // the same two conditions. A selected video that the user has merely
  // paused still counts: pausing only stops its own frames, through
  // _paused below, rather than tearing the whole engine down and
  // losing the frame on screen.
  readonly property bool _decoding: root.available && root.path !== "" && root.visible && root.windowActive && (root._animated || root.playing)
  // _paused is a selected, non-animated video's own local pause state:
  // the engine stays loaded and its last frame stays on screen, only
  // its own playback stops. Reset whenever a new engine loads, so
  // resuming a video stopped by scrolling away or the window losing
  // focus always starts playing again rather than silently frozen.
  property bool _paused: false
  // _engine is the loaded VideoRuntime, as a plain var: see
  // VoiceNoteController for why this is never Loader's own typed item.
  property var _engine: _runtime.item
  // _unavailable is true once a fetch for this video is known to have
  // failed and there is no thumbnail to fall back to either, the same
  // rule PhotoView applies to a photo.
  readonly property bool _unavailable: !root.video.thumb && root.failed && root.path === ""

  implicitWidth: Math.min(root.maxWidth, Style.space(320))
  implicitHeight: Math.min(root.implicitWidth * root._aspect, Style.space(400))

  // An animated GIF asks for its file the moment it is shown, exactly
  // like a photo's own thumbnail-to-full-image fetch; an ordinary video
  // never does, since videos can be large.
  Component.onCompleted: if (root.visible && root._animated && !root.path) root.wanted()
  onVisibleChanged: if (root.visible && root._animated && !root.path) root.wanted()
  onPathChanged: if (root.visible && root._animated && !root.path) root.wanted()

  Loader {
    id: _runtime
    anchors.fill: parent
    active: root._decoding
    source: root.runtimeSource

    onLoaded: {
      root._paused = false;
      _runtime.item.source = root.path;
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
  // decoding: before a click, while paused, or for an animated GIF this
  // machine cannot play inline at all.
  Image {
    id: thumb
    objectName: "videoThumb"
    anchors.fill: parent
    visible: !root._decoding && !(root._animated && root._realGifFile && root.path)
    source: root.video.thumb ? "data:image/jpeg;base64," + root.video.thumb : ""
    fillMode: Image.PreserveAspectFit
    asynchronous: true
    smooth: true
  }

  // A real .gif file animates through AnimatedImage even with no
  // QtMultimedia at all: this is the one case a machine without it
  // still gets true inline playback for a "GIF".
  AnimatedImage {
    id: realGif
    objectName: "realGifImage"
    anchors.fill: parent
    visible: !root._decoding && root._animated && root._realGifFile && root.path !== ""
    source: visible ? "file://" + root.path : ""
    playing: visible && root.visible && root.windowActive
    fillMode: Image.PreserveAspectFit
  }

  Rectangle {
    anchors.fill: parent
    visible: (thumb.visible || realGif.visible) && !root.video.thumb && root.path === ""
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
    visible: !root._animated && !root._decoding && !root._unavailable
    text: "▶"
    textFormat: Text.PlainText
    color: Theme.foreground
    style: Text.Outline
    styleColor: Util.alpha(Theme.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.heading }
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

  // _togglePause starts or stops a selected video's own playback
  // without deselecting it: the engine stays loaded and its last frame
  // stays on screen either way.
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

  MouseArea {
    anchors.fill: parent
    visible: !root._animated && !controls.visible
    enabled: root.visible
    cursorShape: Qt.PointingHandCursor
    onClicked: root.available ? root.toggled() : root.opened()
  }
}
