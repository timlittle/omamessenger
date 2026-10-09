import QtQuick
import qs.Commons

// A sticker in a message: a small, borderless static image, never
// animated (an animated WebP or video sticker still only ever shows its
// first frame here; a Telegram Lottie or WebM sticker a connector
// cannot show as a still image at all leaves path and thumb both empty,
// see domain.Media's own doc comment on the Go side). Falls back to the
// sticker's own emoji, in large type, when there is neither a thumbnail
// nor a downloaded image yet.
Item {
  id: root

  // sticker is the message's sticker media: kind, thumb (base64) and
  // emoji.
  required property var sticker
  // path is where the full image was downloaded, or "" until then.
  property string path: ""

  // wanted asks for the full sticker image to be downloaded.
  signal wanted()

  // _size is the sticker's fixed display size: small and square, with
  // no bubble around it, unlike a photo or file.
  readonly property real _size: Style.space(128)
  // _hasImage is true once there is something to draw: a downloaded
  // image or a thumbnail. False means only the emoji fallback applies.
  readonly property bool _hasImage: root.path !== "" || !!root.sticker.thumb

  implicitWidth: root._size
  implicitHeight: root._size

  // A fetchable sticker (FileName set on the Go side) asks for its
  // image as soon as it is shown, the same as a photo; one this UI can
  // only show as its emoji or thumbnail never has a path to ask for.
  Component.onCompleted: if (root.visible && !root.path) root.wanted()
  onVisibleChanged: if (root.visible && !root.path) root.wanted()

  Image {
    id: image

    objectName: "stickerImage"
    anchors.fill: parent
    visible: root._hasImage
    // Always the helper's own downloaded file or an embedded base64
    // thumb; never a remote URL.
    source: root.path ? "file://" + root.path : (root.sticker.thumb ? "data:image/png;base64," + root.sticker.thumb : "")
    fillMode: Image.PreserveAspectFit
    asynchronous: true
    smooth: true
  }

  Text {
    objectName: "stickerEmoji"
    anchors.centerIn: parent
    visible: !root._hasImage
    text: root.sticker.emoji || "🏷️"
    // The sticker's emoji fallback is sender-controlled: PlainText.
    textFormat: Text.PlainText
    font.pixelSize: root._size * 0.6
  }
}
