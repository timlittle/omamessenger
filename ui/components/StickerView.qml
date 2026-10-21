import QtQuick
import qs.Commons
import "../theme"
import "../lib/Links.js" as Links

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
  // failedReason is a failed automatic fetch's safe reason category
  // (see server/errors.go), "" otherwise; "too-large" means the
  // automatic fetch this sticker's own visibility asked for was
  // refused only for being over that fetch's own, small cap (see
  // _tooLargeAuto).
  property string failedReason: ""

  // wanted asks for the full sticker image to be downloaded.
  signal wanted()
  // retryRequested asks for the full sticker image to be downloaded
  // deliberately, bypassing the automatic-fetch cap wanted's own fetch
  // applies: the tap-to-load action _tooLargeAuto offers.
  signal retryRequested()

  // _size is the sticker's fixed display size: small and square, with
  // no bubble around it, unlike a photo or file.
  readonly property real _size: Style.space(128)
  // _hasImage is true once there is something to draw: a downloaded
  // image or a thumbnail. False means only the emoji fallback applies.
  readonly property bool _hasImage: root.path !== "" || !!root.sticker.thumb
  // _tooLargeAuto is true once the automatic fetch this sticker's own
  // visibility asked for was refused only for being over its own,
  // small cap: never a permanent failure, so a click still asks for
  // the full image, deliberately (see retryRequested).
  readonly property bool _tooLargeAuto: root.path === "" && root.failedReason === "too-large"

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
    source: root.path ? Links.fileURL(root.path) : (root.sticker.thumb ? "data:image/png;base64," + root.sticker.thumb : "")
    fillMode: Image.PreserveAspectFit
    // Bounded to this sticker's own fixed box (see PhotoView.qml's own
    // sourceSize for why).
    sourceSize.width: root._size
    sourceSize.height: root._size
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

  // tooLargeLabel marks a sticker refused only for being over the
  // automatic-fetch cap as tappable, rather than leaving it looking
  // the same as the emoji fallback forever.
  Text {
    objectName: "stickerTooLargeLabel"
    anchors { bottom: parent.bottom; left: parent.left; right: parent.right; margins: Theme.spacing.xxs }
    visible: root._tooLargeAuto
    horizontalAlignment: Text.AlignHCenter
    wrapMode: Text.Wrap
    text: "Tap to load"
    textFormat: Text.PlainText
    color: Theme.foreground
    font { family: Theme.font.family; pixelSize: Theme.font.caption }
  }

  MouseArea {
    objectName: "stickerTooLargeArea"
    anchors.fill: parent
    visible: root._tooLargeAuto
    cursorShape: Qt.PointingHandCursor
    onClicked: root.retryRequested()
  }
}
