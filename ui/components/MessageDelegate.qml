import QtQuick
import qs.Commons
import "../theme"
import "../lib/Format.js" as Format

// One message row in a conversation: an optional day separator, the sender
// name for a grouped incoming message, the bubble, and a time/status row.
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

  // retry asks the caller to resend this message after a send failure.
  signal retry(string id)

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
      width: parent.width
      height: bubble.height

      // The bubble wraps its text: as wide as the text needs, up to 72% of
      // the row, after which the text wraps.
      Rectangle {
        id: bubble

        readonly property real padding: Theme.spacing.sm
        readonly property real maxTextWidth: root.width * 0.72 - padding * 2

        objectName: "bubble"
        width: Math.max(body.width, meta.implicitWidth) + padding * 2
        height: body.height + meta.implicitHeight + Theme.spacing.xxs + padding * 2
        radius: Style.cornerRadius
        color: root.message.outgoing ? Util.alpha(Color.accent, 0.22) : Util.alpha(Color.foreground, 0.06)
        anchors.right: root.message.outgoing ? parent.right : undefined
        anchors.left: root.message.outgoing ? undefined : parent.left

        // The text's unwrapped width. TextEdit's own implicit width follows
        // its wrapped width, so it cannot size the bubble.
        TextMetrics {
          id: natural

          font: body.font
          text: root.message.text
        }

        TextEdit {
          id: body

          objectName: "body"

          x: bubble.padding
          y: bubble.padding
          width: Math.min(Math.ceil(natural.advanceWidth) + 1, bubble.maxTextWidth)
          readOnly: true
          selectByMouse: true
          wrapMode: TextEdit.Wrap
          textFormat: TextEdit.RichText
          text: Format.linkify(Format.escapeHtml(root.message.text))
          color: Color.foreground
          font { family: Theme.font.family; pixelSize: Theme.font.body }
          onLinkActivated: link => Qt.openUrlExternally(link)
        }

        Row {
          id: meta

          y: body.y + body.height + Theme.spacing.xxs
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
            visible: root.showStatus
            text: Format.statusGlyph(root.message.status)
            color: root.message.status === "read" ? Color.accent : Util.alpha(Color.foreground, 0.5)
            font { family: Theme.font.family; pixelSize: Theme.font.caption }
          }
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
