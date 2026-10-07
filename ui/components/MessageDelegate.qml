import QtQuick
import QtQuick.Controls
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format
import "../lib/Timeline.js" as Timeline

// One message row in a conversation: an optional day separator, the sender
// name for a grouped incoming message, the bubble with its text and any
// link preview, photo, video or file, and a time/status row.
// A failed outgoing message shows a retry line instead of a status glyph.
Item {
  id: root

  // message carries id, senderName, text, outgoing, status and created,
  // as the helper sends them.
  required property var message
  // annotation is one entry from Timeline.annotate: showDay, dayLabel,
  // showSender and groupedWithOlder.
  required property var annotation
  // isGroup is true for a group chat, where incoming messages show a name.
  property bool isGroup: false
  // nowMs is the current time, passed through to time formatting.
  property real nowMs: 0
  // failed is true for an outgoing message the service could not send.
  readonly property bool failed: root.message.outgoing && root.message.status === "failed"
  // showStatus is true when the delivery glyph should be drawn.
  readonly property bool showStatus: root.message.outgoing && !root.failed
  // media is what the message carries besides its text, or null.
  readonly property var media: Timeline.media(root.message)
  // quote is the message this one replies to, or null.
  readonly property var quote: Timeline.replyTo(root.message)

  // retry asks the caller to resend this message after a send failure.
  signal retry(string id)
  // mediaWanted asks for this message's photo to be downloaded.
  signal mediaWanted(string id)
  // mediaOpen asks for this message's photo, video or file to be opened.
  signal mediaOpen(string id)
  // replyRequested asks the caller to start replying to this message.
  signal replyRequested(string id)
  // quoteOpened asks the caller to scroll to the message this one quotes.
  signal quoteOpened(string remoteId)

  width: ListView.view ? ListView.view.width : implicitWidth
  implicitHeight: column.implicitHeight

  Column {
    id: column
    width: parent.width
    topPadding: root.annotation.groupedWithOlder ? Theme.spacing.xxs : Theme.spacing.md
    spacing: Theme.spacing.xxs

    Text {
      width: parent.width
      visible: root.annotation.showDay
      horizontalAlignment: Text.AlignHCenter
      text: root.annotation.dayLabel
      color: Util.alpha(Color.foreground, 0.5)
      font.family: Theme.font.family
      font.pixelSize: Theme.font.caption
    }

    Text {
      x: Theme.spacing.md
      visible: root.annotation.showSender
      text: root.message.senderName
      color: Color.accent
      font.family: Theme.font.family
      font.pixelSize: Theme.font.caption
      font.weight: Font.DemiBold
    }

    Item {
      id: bubbleRow
      width: parent.width
      height: bubble.height

      // A passive hover handler shows the reply button without taking
      // the bubble's own clicks, the same way the list row's pin glyph
      // tracks hover beside its own MouseArea.
      HoverHandler {
        id: rowHover
      }

      // The bubble wraps its text: as wide as the text needs, up to 72% of
      // the row, after which the text wraps.
      Rectangle {
        id: bubble

        readonly property real padding: Theme.spacing.sm
        readonly property real maxTextWidth: root.width * 0.72 - padding * 2

        objectName: "bubble"
        width: Math.max(content.width, meta.implicitWidth) + padding * 2
        height: meta.y + meta.implicitHeight + padding
        radius: Style.cornerRadius
        color: root.message.outgoing ? Util.alpha(Color.accent, 0.22) : Util.alpha(Color.foreground, 0.06)
        anchors.right: root.message.outgoing ? parent.right : undefined
        anchors.left: root.message.outgoing ? undefined : parent.left

        // The widest line's unwrapped width. TextEdit's own implicit width
        // follows its wrapped width, so it cannot size the bubble.
        TextMetrics {
          id: natural

          font: body.font
          text: Format.longestLine(body.caption)
        }

        // The text, then any link preview or photo; parts a message lacks
        // take no space.
        Column {
          id: content

          x: bubble.padding
          y: bubble.padding
          spacing: Theme.spacing.xs

          ReplyQuote {
            objectName: "replyQuote"
            width: Math.min(implicitWidth, bubble.maxTextWidth)
            visible: root.quote !== null
            reply: root.quote ?? ({})
            onOpened: remoteId => root.quoteOpened(remoteId)
          }

          TextEdit {
            id: body

            // caption leaves out the label a photo stands in for.
            readonly property string caption: Format.caption(root.message.text, root.media)

            objectName: "body"
            visible: body.caption !== ""
            width: Math.min(Math.ceil(natural.advanceWidth) + 1, bubble.maxTextWidth)
            readOnly: true
            selectByMouse: true
            wrapMode: TextEdit.Wrap
            textFormat: TextEdit.RichText
            text: Format.messageHtml(body.caption, Color.accent)
            color: Color.foreground
            font { family: Theme.font.family; pixelSize: Theme.font.body }
            onLinkActivated: link => Qt.openUrlExternally(link)
          }

          LinkPreview {
            objectName: "linkPreview"
            width: Math.min(implicitWidth, bubble.maxTextWidth)
            visible: root.media !== null && root.media.kind === "link"
            preview: root.media ?? ({})
            onOpened: url => Qt.openUrlExternally(url)
          }

          PhotoView {
            objectName: "photoView"
            maxWidth: bubble.maxTextWidth
            width: implicitWidth
            height: implicitHeight
            visible: root.media !== null && (root.media.kind === "photo" || root.media.kind === "video")
            photo: root.media ?? ({})
            path: root.message.mediaPath ?? ""
            onWanted: root.mediaWanted(root.message.id)
            onOpened: root.mediaOpen(root.message.id)
          }

          FileView {
            objectName: "fileView"
            width: Math.min(implicitWidth, bubble.maxTextWidth)
            visible: root.media !== null && root.media.kind === "file"
            file: root.media ?? ({})
            onOpened: root.mediaOpen(root.message.id)
          }
        }

        Row {
          id: meta

          y: content.y + content.height + Theme.spacing.xxs
          spacing: Theme.spacing.xs
          layoutDirection: root.message.outgoing ? Qt.RightToLeft : Qt.LeftToRight
          anchors.right: root.message.outgoing ? parent.right : undefined
          anchors.left: root.message.outgoing ? undefined : parent.left
          anchors.leftMargin: bubble.padding
          anchors.rightMargin: bubble.padding

          Text {
            text: Format.timeLabel(root.message.created, root.nowMs)
            color: Util.alpha(Color.foreground, 0.5)
            font { family: Theme.font.family; pixelSize: Theme.font.caption }
          }

          Text {
            visible: root.message.edited === true
            text: "edited"
            color: Util.alpha(Color.foreground, 0.5)
            font { family: Theme.font.family; pixelSize: Theme.font.caption }
          }

          Text {
            visible: root.showStatus
            text: Format.statusGlyph(root.message.status)
            color: root.message.status === "read" ? Color.accent : Util.alpha(Color.foreground, 0.5)
            font { family: Theme.font.family; pixelSize: Theme.font.caption }
          }
        }
      }

      // replyButton appears on hover over the bubble, to start a reply
      // to this message; it sits just outside the bubble, on the side
      // nearer the middle of the row.
      Text {
        id: replyButton
        objectName: "replyButton"
        visible: rowHover.hovered
        text: "↩"
        color: Util.alpha(Color.foreground, 0.6)
        font.family: Theme.font.family
        font.pixelSize: Theme.font.body
        y: bubble.y + (bubble.height - height) / 2
        x: root.message.outgoing ? bubble.x - width - Theme.spacing.xs : bubble.x + bubble.width + Theme.spacing.xs

        HoverHandler {
          id: replyButtonHover
        }
        ToolTip.visible: replyButtonHover.hovered
        ToolTip.text: "Reply"
        ToolTip.delay: 500

        MouseArea {
          anchors.fill: parent
          cursorShape: Qt.PointingHandCursor
          onClicked: root.replyRequested(root.message.id)
        }
      }
    }

    Item {
      width: parent.width
      height: retryText.height
      visible: root.failed

      Text {
        id: retryText
        anchors.right: parent.right
        text: "Not sent · r to retry"
        color: Color.urgent
        font.family: Theme.font.family
        font.pixelSize: Theme.font.caption
      }

      MouseArea {
        anchors.fill: retryText
        cursorShape: Qt.PointingHandCursor
        onClicked: root.retry(root.message.id)
      }
    }
  }
}
