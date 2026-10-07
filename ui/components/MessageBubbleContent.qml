pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// The bubble's content stack: a reply quote if this message answers one,
// then its text, then any link preview, photo or file it carries. Split
// out of MessageDelegate to keep that file within the size guideline.
// Parts a message lacks take no space.
Column {
  id: root

  // message carries text and mediaPath, as MessageDelegate reads them.
  // It, media and quote can all be reset straight to a QML null while
  // the delegate this belongs to is destroyed, so every binding below
  // guards them with "&&" or a ternary rather than trusting required to
  // mean non-null for the binding's whole lifetime.
  required property var message
  // media is what the message carries besides its text, or null.
  required property var media
  // quote is the message this one replies to, or null.
  required property var quote
  // maxTextWidth caps how wide the text, link preview, photo or file may
  // grow before wrapping or eliding.
  required property real maxTextWidth

  // mediaWanted asks for this message's photo to be downloaded.
  signal mediaWanted()
  // mediaOpen asks for this message's photo, video or file to be opened.
  signal mediaOpen()
  // quoteOpened asks the caller to scroll to the message this one quotes.
  signal quoteOpened(string remoteId)

  spacing: Theme.spacing.xs

  ReplyQuote {
    objectName: "replyQuote"
    width: Math.min(implicitWidth, root.maxTextWidth)
    visible: !!root.quote
    reply: root.quote ? root.quote : ({})
    onOpened: remoteId => root.quoteOpened(remoteId)
  }

  TextEdit {
    id: body

    // caption leaves out the label a photo stands in for.
    readonly property string caption: root.message ? Format.caption(root.message.text, root.media) : ""

    objectName: "body"
    visible: body.caption !== ""
    width: Math.min(Math.ceil(natural.advanceWidth) + 1, root.maxTextWidth)
    readOnly: true
    selectByMouse: true
    wrapMode: TextEdit.Wrap
    textFormat: TextEdit.RichText
    text: Format.messageHtml(body.caption, Color.accent)
    color: Color.foreground
    font { family: Theme.font.family; pixelSize: Theme.font.body }
    onLinkActivated: link => Qt.openUrlExternally(link)
  }

  // The widest line's unwrapped width. TextEdit's own implicit width
  // follows its wrapped width, so it cannot size the bubble.
  TextMetrics {
    id: natural

    font: body.font
    text: Format.longestLine(body.caption)
  }

  LinkPreview {
    objectName: "linkPreview"
    width: Math.min(implicitWidth, root.maxTextWidth)
    visible: root.media && root.media.kind === "link"
    preview: root.media ? root.media : ({})
    onOpened: url => Qt.openUrlExternally(url)
  }

  PhotoView {
    objectName: "photoView"
    maxWidth: root.maxTextWidth
    width: implicitWidth
    height: implicitHeight
    visible: root.media && (root.media.kind === "photo" || root.media.kind === "video")
    photo: root.media ? root.media : ({})
    path: root.message && root.message.mediaPath ? root.message.mediaPath : ""
    onWanted: root.mediaWanted()
    onOpened: root.mediaOpen()
  }

  FileView {
    objectName: "fileView"
    width: Math.min(implicitWidth, root.maxTextWidth)
    visible: root.media && root.media.kind === "file"
    file: root.media ? root.media : ({})
    onOpened: root.mediaOpen()
  }
}
