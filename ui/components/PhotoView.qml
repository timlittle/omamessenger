import QtQuick
import qs.Commons
import "../theme"
import "../lib/Links.js" as Links

// A photo in a message, at its shape. It shows its blurred preview until
// the full image is downloaded, which it asks for as soon as it is
// shown; if the full image then fails to load, or its fetch is known to
// have failed outright, it falls back to the preview, or a quiet
// message naming the file when there is no preview either. Without that
// second check a photo with neither a preview nor a successful download
// would stay an empty box forever, since nothing ever attempts to load
// an empty image source. A click opens the in-app photo viewer. A video
// is InlineVideo instead, which plays in place rather than only ever
// opening externally.
Item {
  id: root

  // photo is the message's photo media: width, height and thumb, a
  // base64 JPEG preview.
  required property var photo
  // path is where the full image was downloaded, or "" until then.
  property string path: ""
  // failed is true once a fetch for this photo's full image has
  // definitely failed, as opposed to simply not having happened yet.
  property bool failed: false
  // maxWidth is the widest the photo may be drawn.
  property real maxWidth: Style.space(320)

  // wanted asks for the full photo to be downloaded.
  signal wanted()
  // opened asks for the photo to be opened in the in-app viewer,
  // downloading it first if need be.
  signal opened()

  // _aspect is height over width, square when the size is unknown.
  readonly property real _aspect: root.photo.width > 0 ? root.photo.height / root.photo.width : 1

  // _loadFailed latches once the full image at path fails to load, such
  // as a path left over from a file since moved or deleted; resetting it
  // only on a new path means a transient error does not flap back to
  // showing the broken file the next time this delegate is reused.
  property bool _loadFailed: false

  // _retriedForPath is true once a load failure for the current path
  // has already asked for a fresh one, so a path that keeps failing
  // does not ask again and again; resetting it only on a new path lets
  // a genuinely new failure ask once more.
  property bool _retriedForPath: false

  // _unavailable is true once the full image is known not to be coming
  // (it failed to load, or its fetch itself failed) and there is no
  // thumb to fall back to either, so the bubble shows a quiet message
  // instead of staying blank.
  readonly property bool _unavailable: !root.photo.thumb && (root._loadFailed || root.failed)

  implicitWidth: Math.min(root.maxWidth, Style.space(320))
  implicitHeight: Math.min(root.implicitWidth * root._aspect, Style.space(400))

  // Each time a photo row is shown without its image, it asks for it;
  // rows are recycled as the list scrolls, so this follows what is on
  // screen. A path cleared out from under it - the helper retiring a
  // sent photo's attachment drops a now-stale cached path the same way
  // - counts as "shown without its image" too, so it asks again right
  // away rather than waiting for the next visibility change.
  Component.onCompleted: if (root.visible && !root.path) root.wanted()
  onVisibleChanged: if (root.visible && !root.path) root.wanted()
  onPathChanged: {
    root._loadFailed = false;
    root._retriedForPath = false;
    if (root.visible && !root.path) root.wanted();
  }

  Image {
    id: image

    objectName: "photoImage"
    anchors.fill: parent
    // Always the helper's own downloaded file or an embedded base64
    // thumb; never a remote URL.
    source: root.path && !root._loadFailed ? Links.fileURL(root.path) : (root.photo.thumb ? "data:image/jpeg;base64," + root.photo.thumb : "")
    fillMode: Image.PreserveAspectFit
    // Bounded to this bubble's own box, never the file's own declared
    // size: the helper's imagesafe package already caps what a photo
    // or sticker can declare, but a bounded sourceSize means Qt never
    // even tries to allocate a bitmap larger than what is actually
    // drawn, for a received image specifically (see docs/decisions.md).
    sourceSize.width: root.implicitWidth
    sourceSize.height: root.implicitHeight
    asynchronous: true
    // Scaling a tiny preview up smoothly keeps it a soft blur.
    smooth: true
    // A path that no longer resolves to a real file - the local copy
    // swept, or moved into the cache under a different name the model
    // was never told about - must not stay a permanently broken image:
    // asking once for a fresh path either recovers it or, once that
    // fails too, leaves the thumbnail or "Photo unavailable" showing
    // instead, same as any other failed fetch.
    onStatusChanged: {
      if (status !== Image.Error) return;
      root._loadFailed = true;
      if (root._retriedForPath) return;
      root._retriedForPath = true;
      root.wanted();
    }
  }

  // A photo that failed to load and has no thumb either still needs a
  // quiet background rather than an empty box.
  Rectangle {
    anchors.fill: parent
    visible: root._unavailable
    color: Util.alpha(Theme.foreground, 0.08)
  }

  Text {
    objectName: "unavailableLabel"
    anchors.centerIn: parent
    width: parent.width - Theme.spacing.sm * 2
    visible: root._unavailable
    horizontalAlignment: Text.AlignHCenter
    wrapMode: Text.Wrap
    text: root.photo.fileName ? "Photo unavailable · " + root.photo.fileName : "Photo unavailable"
    // fileName is sender-controlled: PlainText.
    textFormat: Text.PlainText
    color: Util.alpha(Theme.foreground, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  MouseArea {
    anchors.fill: parent
    enabled: root.path !== "" && !root._loadFailed
    cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
    onClicked: root.opened()
  }
}
