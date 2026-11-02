import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format
import "../lib/ImageBounds.js" as ImageBounds
import "../lib/Media.js" as Media

// A video or "GIF video" in a message, once downloaded. Nothing here
// ever fetches or decodes without a click: a video the sending
// service marked as a silent, looping "GIF" (see domain.Media.Animated)
// shows its thumbnail and a play mark exactly like any other video,
// never autoplaying on its own - that flag is fully sender-controlled,
// and the fetch and decode it used to trigger for free is exactly the
// risk this component now closes. A click downloads the file if it is
// not here yet, asks the helper to prepare its in-window play copy
// (see playPath below), and only then plays it in place: looping and
// muted for a "GIF" video, with play/pause, a draggable seek bar and
// a mute button for an ordinary one. Either kind stops decoding - the
// engine is torn down, not merely paused - once this bubble scrolls
// out of view, the window loses focus, or another video or the
// full-size viewer takes over the one decoding slot every video
// shares (see VideoController), so a long scroll-back through a chat
// full of videos never keeps more than one of them decoding at once.
//
// This never loads a sender's own bytes into any engine at all: the
// one file it ever plays is playPath, the file the helper's own
// sandboxed transcoder produced (see docs/decisions.md). available is
// false when QtMultimedia itself could not be loaded (see
// VideoController); playPath is "" until a play fetch has produced
// one, which falls back to opening externally instead of decoding in
// place, exactly as it did before this component existed.
Item {
  id: root

  // video is the message's video media: kind, width, height, duration,
  // thumb, animated and fileName.
  required property var video
  // path is where the full file was downloaded, or "" until then:
  // read only to decide whether anything has even been fetched yet
  // (see _unavailable) and as the fallback this opens externally once
  // available or playPath says in-window playback is not an option.
  property string path: ""
  // playPath is the in-window play copy a play fetch produced (see
  // media.fetch's playPath in docs/api.md), or "" before one has
  // answered or succeeded. This is the only file this component ever
  // decodes; a sender's own downloaded bytes at path are never handed
  // to the video engine directly.
  property string playPath: ""
  // failed is true once a fetch for this video has definitely failed.
  property bool failed: false
  // preparing is true while a play fetch for this video is actually
  // in flight: re-encoding a long clip can take a few seconds, so the
  // play mark becomes a "Preparing…" label instead until playPath
  // answers, one way or another.
  property bool preparing: false
  // available is false when in-window playback cannot work at all:
  // see VideoController.available.
  property bool available: true
  // playing is true while this is the one video, of the single slot
  // every video shares (see VideoController), actually decoding right
  // now - an animated "GIF" included, which no longer decodes on its
  // own just because it is visible.
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
  // expandable shows a click target that asks to open this video or
  // GIF full size (see expandRequested); true by default for the
  // message bubble's own use, false when this is embedded somewhere
  // that already is the full-size view, such as the in-app viewer
  // itself, which would otherwise offer to expand into a copy of
  // itself.
  property bool expandable: true

  // toggled asks the caller to select, deselect or pause this video: a
  // click on a video that is not yet downloaded or selected asks the
  // caller to fetch it, validate it, and only then select it for
  // in-window playback - or, failing that validation, open it
  // externally instead (see MediaController.toggleVideo); the
  // component itself never knows which of those a click will turn out
  // to be ahead of time.
  signal toggled()
  // muteToggled asks the caller to flip the shared mute preference.
  signal muteToggled()
  // opened asks for this video to be opened externally instead: the
  // fallback a click takes while available is false, exactly what it
  // did before in-window playback existed.
  signal opened()
  // expandRequested asks the caller to show this video or GIF full
  // size in the in-app viewer: reachable once this is the one video
  // already selected and decoding, over the frame itself rather than
  // its controls, so a first click still starts it inline exactly as
  // it always did.
  signal expandRequested()

  // _animated is true for a video the service marked as a silent,
  // looping "GIF".
  readonly property bool _animated: Media.isAnimated(root.video)
  // _verifiedWidth and _verifiedHeight are the thumbnail's own real,
  // decoder-confirmed size (see ImageBounds.verifiedDimensions), for
  // the static thumb Image below to bound its decode by: never
  // root.video.width/height, the sender's own claim, which a
  // differential review found need not match the thumbnail's real
  // shape. This always reads the thumbnail's own verified size, never
  // the full file's - the file itself is never decoded as an image at
  // all any more, only ever played from its own play copy (see
  // playPath below).
  readonly property int _verifiedWidth: ImageBounds.verifiedDimensions(root.video, false).width
  readonly property int _verifiedHeight: ImageBounds.verifiedDimensions(root.video, false).height
  // _aspect is height over width, square when the size is unknown.
  readonly property real _aspect: root.video.width > 0 ? root.video.height / root.video.width : 1
  // _decoding is true while this bubble should actually hold the
  // QtMultimedia engine: the one video the caller selected (playing),
  // with a play copy ready, on screen and with the window active. A
  // selected video that the user has merely paused still counts:
  // pausing only stops its own frames, through _paused below, rather
  // than tearing the whole engine down and losing the frame on screen.
  readonly property bool _decoding: root.available && root.playPath !== "" && root.visible && root.windowActive && root.playing
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
  // _expandable is true while a click on this video's own frame should
  // ask to open the full-size viewer instead of toggling it again:
  // once it is the one selected and playing, so a first click still
  // starts it inline exactly as it always did, whether this is an
  // animated "GIF" or an ordinary video.
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
    // Fit inside this bubble's own box, computed from the thumbnail's
    // own verified width and height (see PhotoView.qml's own
    // sourceSize for why the box alone is not enough): never
    // root.video.width/height, the sender's own claim.
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

  // Asks to open the full-size viewer, once this video is the one
  // selected and playing: a real GIF's whole frame, which has no
  // controls row to leave room for, or an ordinary video's frame
  // above its own controls row, left clickable for pause, seek and
  // mute.
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

  // Starts, selects or opens this video: every first click, before it
  // is the one selected and playing, lands here rather than on
  // expandArea above, which only claims the bubble once _expandable
  // is already true - the same switch controls.visible makes for an
  // ordinary video, generalised to cover a real GIF too, which never
  // shows a controls row at all.
  MouseArea {
    anchors.fill: parent
    visible: !controls.visible && !root._expandable
    enabled: root.visible
    cursorShape: Qt.PointingHandCursor
    onClicked: root.available ? root.toggled() : root.opened()
  }
}
