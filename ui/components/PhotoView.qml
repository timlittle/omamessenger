import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// A photo or video in a message, at its shape. A photo shows its blurred
// preview until the full image is downloaded, which it asks for as soon as
// it is shown. A video shows its preview with a play mark and its length,
// and downloads only when clicked, since videos can be large. A click opens
// either in the user's viewer.
Item {
  id: root

  // photo is the message's photo or video media: kind, width, height,
  // duration and thumb, a base64 JPEG preview.
  required property var photo
  // path is where the full image was downloaded, or "" until then.
  property string path: ""
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

  implicitWidth: Math.min(root.maxWidth, Style.space(320))
  implicitHeight: Math.min(root.implicitWidth * root._aspect, Style.space(400))

  // Each time a photo row is shown without its image, it asks for it;
  // rows are recycled as the list scrolls, so this follows what is on
  // screen.
  Component.onCompleted: if (root.visible && !root.path && !root._video) root.wanted()
  onVisibleChanged: if (root.visible && !root.path && !root._video) root.wanted()

  Image {
    id: image

    objectName: "photoImage"
    anchors.fill: parent
    source: root.path && !root._video ? "file://" + root.path : (root.photo.thumb ? "data:image/jpeg;base64," + root.photo.thumb : "")
    fillMode: Image.PreserveAspectFit
    asynchronous: true
    // Scaling a tiny preview up smoothly keeps it a soft blur.
    smooth: true
  }

  // A video without a preview still needs something to click.
  Rectangle {
    anchors.fill: parent
    visible: root._video && !root.photo.thumb
    color: Util.alpha(Color.foreground, 0.08)
  }

  Text {
    objectName: "playMark"
    anchors.centerIn: parent
    visible: root._video
    text: "▶"
    color: Color.foreground
    style: Text.Outline
    styleColor: Util.alpha(Color.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.heading }
  }

  Text {
    anchors { right: parent.right; bottom: parent.bottom; margins: Theme.spacing.xs }
    visible: root._video && text !== ""
    text: Format.duration(root.photo.duration)
    color: Color.foreground
    style: Text.Outline
    styleColor: Util.alpha(Color.background, 0.6)
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  MouseArea {
    anchors.fill: parent
    enabled: root.path !== "" || root._video
    cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
    onClicked: root.opened()
  }
}
