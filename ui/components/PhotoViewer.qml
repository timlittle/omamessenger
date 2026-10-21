import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Format.js" as Format
import "../lib/Keymap.js" as Keymap
import "../lib/Links.js" as Links

// Shows a conversation's photo, video or GIF over the whole window.
// Omarchy's Hyprland window rule floats an external viewer too small to
// reach with the keyboard, and focus stays on this window, so a photo or
// video opened there gets stuck with no way to close it; this shows it
// here instead, where Escape, the window's own close button and every
// other shortcut still work. While a photo's full image is still
// downloading it shows the same blurred preview the message bubble
// already has; a video or GIF plays through FullVideoPlayer once its
// file is here, the same way the bubble's own InlineVideo does but sized
// to fill the window.
Item {
  id: root

  // open shows the viewer when true.
  property bool open: false
  // photo is the open message's media: kind, width, height, thumb,
  // fileName and animated, or null while nothing is open. Named for the
  // photo case this started as; a video or GIF is just as much "photo"
  // here, since every binding below reads it generically.
  property var photo: null
  // path is where the full file was downloaded, or "" until then.
  property string path: ""
  // decodeAs is the helper's own verdict on how to decode the open
  // video (see InlineVideo.decodeAs), or "" for a photo.
  property string decodeAs: ""
  // videoAvailable is false when in-window video playback cannot work
  // at all on this machine; a real GIF file plays regardless. Read only
  // while photo is a video.
  property bool videoAvailable: true
  // videoMuted mutes a playing video, shared with every other video in
  // the window.
  property bool videoMuted: true
  // routeKey is the panel's key router, so Escape and the photo-stepping
  // keys work while this holds keyboard focus, the same as every other
  // full-window dialog here.
  property var routeKey: null
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), read for the external-open button.
  property var bindings: Keymap.BINDINGS

  // closed asks the caller to leave the viewer: the close button, or a
  // click on the backdrop outside the photo or video.
  signal closed()
  // openExternally asks the caller to open the photo or video in the
  // user's own application and leave the in-app view.
  signal openExternally()
  // videoMuteToggled asks the caller to flip the shared mute preference
  // every video reads.
  signal videoMuteToggled()

  // _isVideo is true while photo is actually a video or GIF, which
  // plays through FullVideoPlayer instead of the plain Image below.
  readonly property bool _isVideo: !!root.photo && root.photo.kind === "video"

  objectName: "photoViewer"
  anchors.fill: parent
  visible: root.open
  focus: true

  Keys.priority: Keys.BeforeItem
  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
  }

  // Focus waits a turn of the event loop: an item still hidden when open
  // changes cannot take focus.
  onOpenChanged: if (root.open) Qt.callLater(root.forceActiveFocus)

  Rectangle {
    id: backdrop
    objectName: "backdrop"
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
      onClicked: root.closed()
    }
  }

  Item {
    id: stage
    anchors.fill: parent
    anchors.margins: Theme.spacing.xl

    Image {
      id: image
      objectName: "photoImage"
      anchors.fill: parent
      visible: !root._isVideo
      // Always the helper's own downloaded file or an embedded base64
      // thumb; never a remote URL.
      source: root.path ? Links.fileURL(root.path) : (root.photo && root.photo.thumb ? "data:image/jpeg;base64," + root.photo.thumb : "")
      fillMode: Image.PreserveAspectFit
      // Bounded to the viewer's own stage, never the file's own
      // declared size (see PhotoView.qml's own sourceSize for why).
      sourceSize.width: Math.max(1, stage.width)
      sourceSize.height: Math.max(1, stage.height)
      asynchronous: true
      // Scaling a tiny preview up smoothly keeps it a soft blur, same as
      // the bubble's own photo view.
      smooth: true

      // Clicks on the photo itself stay on the photo; only the backdrop
      // around it, sized to the painted image rather than this item's
      // whole letterboxed bounds, closes the viewer.
      MouseArea {
        anchors.centerIn: parent
        width: image.paintedWidth
        height: image.paintedHeight
      }
    }

    FullVideoPlayer {
      id: videoPlayer
      objectName: "fullVideoPlayer"
      anchors.fill: parent
      visible: root._isVideo
      video: root.photo ? root.photo : ({})
      path: root.path
      decodeAs: root.decodeAs
      available: root.videoAvailable
      muted: root.videoMuted
      onMuteToggled: root.videoMuteToggled()
    }

    Text {
      objectName: "loadingLabel"
      anchors.centerIn: parent
      visible: !root._isVideo && root.path === ""
      text: "Loading…"
      textFormat: Text.PlainText
      color: Util.alpha(Theme.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.body }
    }
  }

  RowLayout {
    anchors { top: parent.top; left: parent.left; right: parent.right; margins: Theme.spacing.md }
    spacing: Theme.spacing.sm

    Text {
      objectName: "sizeLabel"
      Layout.fillWidth: true
      text: root.photo && !root._isVideo ? Format.photoSize(root.photo.width, root.photo.height) : ""
      textFormat: Text.PlainText
      color: Util.alpha(Theme.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
    }

    Ui.Button {
      objectName: "openExternalButton"
      text: root._openLabel() + " (" + Keymap.keyFor("viewer.openExternal", root.bindings) + ")"
      tooltipText: root._openLabel()
      focusable: true
      onClicked: root.openExternally()
    }

    Ui.Button {
      objectName: "closeButton"
      text: "✕"
      tooltipText: "Close"
      focusable: true
      onClicked: root.closed()
    }
  }

  // _openLabel names what the external-open button does: a video or
  // GIF opens in the user's own video player, anything else (a photo)
  // in their own image viewer.
  function _openLabel(): string {
    return root._isVideo ? "Open in your own video player" : "Open in your own image viewer";
  }
}
