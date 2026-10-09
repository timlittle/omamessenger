import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// A photo or video in a message, at its shape. A photo shows its blurred
// preview until the full image is downloaded, which it asks for as soon as
// it is shown; if the full image then fails to load, or its fetch is known
// to have failed outright, it falls back to the preview, or a quiet
// message naming the file when there is no preview either. Without that
// second check a photo with neither a preview nor a successful download
// would stay an empty box forever, since nothing ever attempts to load an
// empty image source. A video shows its preview with a play mark and its
// length, and downloads only when clicked, since videos can be large. A
// click opens either in the user's viewer.
Item {
  id: root

  // photo is the message's photo or video media: kind, width, height,
  // duration and thumb, a base64 JPEG preview.
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
  // opened asks for the photo or video to be opened, downloading it first
  // if need be.
  signal opened()

  // _video is true for a video, which is never downloaded unasked.
  readonly property bool _video: root.photo.kind === "video"

  // _aspect is height over width, square when the size is unknown.
  readonly property real _aspect: root.photo.width > 0 ? root.photo.height / root.photo.width : 1

  // _loadFailed latches once the full image at path fails to load, such
  // as a path left over from a file since moved or deleted; resetting it
  // only on a new path means a transient error does not flap back to
  // showing the broken file the next time this delegate is reused.
  property bool _loadFailed: false

  // _unavailable is true once the full image is known not to be coming
  // (it failed to load, or its fetch itself failed) and there is no
  // thumb to fall back to either, so the bubble shows a quiet message
  // instead of staying blank.
  readonly property bool _unavailable: !root.photo.thumb && (root._loadFailed || root.failed)

  implicitWidth: Math.min(root.maxWidth, Style.space(320))
  implicitHeight: Math.min(root.implicitWidth * root._aspect, Style.space(400))

  // Each time a photo row is shown without its image, it asks for it;
  // rows are recycled as the list scrolls, so this follows what is on
  // screen.
  Component.onCompleted: if (root.visible && !root.path && !root._video) root.wanted()
  onVisibleChanged: if (root.visible && !root.path && !root._video) root.wanted()
  onPathChanged: root._loadFailed = false

  Image {
    id: image

    objectName: "photoImage"
    anchors.fill: parent
    // Always the helper's own downloaded file or an embedded base64
    // thumb; never a remote URL.
    source: root.path && !root._video && !root._loadFailed ? "file://" + root.path : (root.photo.thumb ? "data:image/jpeg;base64," + root.photo.thumb : "")
    fillMode: Image.PreserveAspectFit
    asynchronous: true
    // Scaling a tiny preview up smoothly keeps it a soft blur.
    smooth: true
    onStatusChanged: if (status === Image.Error) root._loadFailed = true
  }

  // A video without a preview, or a photo that failed to load and has no
  // thumb either, still needs a quiet background rather than an empty box.
  Rectangle {
    anchors.fill: parent
    visible: (root._video && !root.photo.thumb) || root._unavailable
    color: Util.alpha(Color.foreground, 0.08)
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
    color: Util.alpha(Color.foreground, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  Text {
    objectName: "playMark"
    anchors.centerIn: parent
    visible: root._video
    text: "▶"
    textFormat: Text.PlainText
    color: Color.foreground
    style: Text.Outline
    styleColor: Util.alpha(Color.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.heading }
  }

  Text {
    anchors { right: parent.right; bottom: parent.bottom; margins: Theme.spacing.xs }
    visible: root._video && text !== ""
    text: Format.duration(root.photo.duration)
    textFormat: Text.PlainText
    color: Color.foreground
    style: Text.Outline
    styleColor: Util.alpha(Color.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  MouseArea {
    anchors.fill: parent
    enabled: (root.path !== "" && !root._loadFailed) || root._video
    cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
    onClicked: root.opened()
  }
}
