import QtQuick
import qs.Commons
import "../theme"
import "../lib/ImageBounds.js" as ImageBounds
import "../lib/Links.js" as Links
import "../lib/Media.js" as Media

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
  // failedReason is that fetch's safe reason category (see
  // server/errors.go), "" when failed is false or the helper gave
  // none; "too-large" means the automatic fetch a row's own visibility
  // asked for was refused only for being over that fetch's own, small
  // cap, never that the photo itself is unavailable (see
  // _tooLargeAuto).
  property string failedReason: ""
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

  // _tooLargeAuto is true once the automatic fetch a row's own
  // visibility asked for was refused only for being over its own,
  // small cap: never a permanent failure, so a click still opens the
  // viewer (see the MouseArea below), which downloads it again,
  // deliberately, rather than leaving it stuck on the blurred preview
  // forever.
  readonly property bool _tooLargeAuto: root.failed && root.failedReason === "too-large" && !root.path

  // _haveFile is whether the full image, rather than its thumbnail,
  // is the one actually shown right now - the same condition image's
  // own source binding below uses - so _verifiedWidth/_verifiedHeight
  // read the right one of the two verified sizes ImageBounds.
  // verifiedDimensions can return. root.photo.verifiedWidth/Height are
  // only ever set from media.fetch's own result for a photo or
  // sticker (see MediaController._applyVerifiedSize and
  // app.servedPath): a path set for any other kind never carries a
  // verified size at all, so requiring one here, not merely a
  // non-empty path, is this view's own check that path is actually
  // the re-encoded display copy a photo's Image may decode - never, by
  // itself, some other kind's raw original left behind by a race
  // between an edit and an in-flight fetch answer (see
  // docs/decisions.md).
  readonly property bool _haveFile: !!root.path && !root._loadFailed && root.photo.verifiedWidth > 0 && root.photo.verifiedHeight > 0
  // _verifiedWidth and _verifiedHeight are the real, decoder-confirmed
  // size to bound the image's own decode by (see
  // ImageBounds.verifiedDimensions): never root.photo.width/height,
  // the sender's own claim, which a differential review found need
  // not match either file's real shape.
  readonly property int _verifiedWidth: ImageBounds.verifiedDimensions(root.photo, root._haveFile).width
  readonly property int _verifiedHeight: ImageBounds.verifiedDimensions(root.photo, root._haveFile).height

  // _unavailable is true once the full image is known not to be coming
  // for good (it failed to load, or its fetch itself failed for a
  // reason other than the automatic-fetch cap) and there is no thumb
  // to fall back to either, so the bubble shows a quiet message
  // instead of staying blank.
  readonly property bool _unavailable: !root.photo.thumb && (root._loadFailed || (root.failed && !root._tooLargeAuto))

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
    // thumb; never a remote URL. Gated on _haveFile, not merely on
    // root.path being set, so this never decodes a path recorded for
    // some other kind (see _haveFile's own comment).
    source: root._haveFile ? Links.fileURL(root.path) : (root.photo.thumb ? "data:" + Media.thumbMime(root.photo.thumb) + ";base64," + root.photo.thumb : "")
    fillMode: Image.PreserveAspectFit
    // Fit inside this bubble's own box, computed from the photo's own
    // verified width and height rather than the box alone: Qt's own
    // PreserveAspectFit/Crop scaling covers a sourceSize box by
    // scaling to the image's longer side, so a sourceSize that does
    // not share the real image's own ratio can still make Qt decode
    // many times the box's own area (see ImageBounds.js and
    // docs/decisions.md). ImageBounds.verifiedDimensions reads the
    // thumbnail's own confirmed size while the full image is still
    // downloading, and the full file's own confirmed size once it is
    // here - never root.photo.width/height, the sender's own claim,
    // which a differential review found need not match either file's
    // real shape.
    sourceSize.width: ImageBounds.boundedSourceSize(root.implicitWidth, root.implicitHeight, root._verifiedWidth, root._verifiedHeight).width
    sourceSize.height: ImageBounds.boundedSourceSize(root.implicitWidth, root.implicitHeight, root._verifiedWidth, root._verifiedHeight).height
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

  // tooLargeLabel marks a photo refused only for being over the
  // automatic-fetch cap as tappable, over the thumbnail when there is
  // one or the quiet background above otherwise, rather than leaving
  // it looking the same as a permanent failure.
  Text {
    objectName: "tooLargeLabel"
    anchors { bottom: parent.bottom; left: parent.left; right: parent.right; margins: Theme.spacing.xs }
    visible: root._tooLargeAuto
    horizontalAlignment: Text.AlignHCenter
    wrapMode: Text.Wrap
    text: "Tap to load full image"
    textFormat: Text.PlainText
    color: Theme.foreground
    font { family: Theme.font.family; pixelSize: Theme.font.caption; weight: Font.DemiBold }
  }

  MouseArea {
    anchors.fill: parent
    enabled: (root.path !== "" && !root._loadFailed) || root._tooLargeAuto
    cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
    onClicked: root.opened()
  }
}
