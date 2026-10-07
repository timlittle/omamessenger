import QtQuick
import qs.Commons
import "../theme"

// A photo in a message: its blurred preview at the photo's shape until the
// full image is downloaded, which it asks for as soon as it is shown. A
// click opens the downloaded image in the user's image viewer.
Item {
  id: root

  // photo is the message's photo media: width, height and thumb, a base64
  // JPEG preview.
  required property var photo
  // path is where the full image was downloaded, or "" until then.
  property string path: ""
  // maxWidth is the widest the photo may be drawn.
  property real maxWidth: Style.space(320)

  // wanted asks for the full image to be downloaded.
  signal wanted()
  // opened asks for the downloaded image to be opened.
  signal opened(string path)

  // _aspect is height over width, square when the size is unknown.
  readonly property real _aspect: root.photo.width > 0 ? root.photo.height / root.photo.width : 1

  implicitWidth: Math.min(root.maxWidth, Style.space(320))
  implicitHeight: Math.min(root.implicitWidth * root._aspect, Style.space(400))

  // Each time a photo row is shown without its image, it asks for it;
  // rows are recycled as the list scrolls, so this follows what is on
  // screen.
  Component.onCompleted: if (root.visible && !root.path) root.wanted()
  onVisibleChanged: if (root.visible && !root.path) root.wanted()

  Image {
    id: image

    objectName: "photoImage"
    anchors.fill: parent
    source: root.path ? "file://" + root.path : (root.photo.thumb ? "data:image/jpeg;base64," + root.photo.thumb : "")
    fillMode: Image.PreserveAspectFit
    asynchronous: true
    // Scaling a tiny preview up smoothly keeps it a soft blur.
    smooth: true
  }

  MouseArea {
    anchors.fill: parent
    enabled: root.path !== ""
    cursorShape: root.path ? Qt.PointingHandCursor : Qt.ArrowCursor
    onClicked: root.opened(root.path)
  }
}
