import QtQuick
import qs.Commons
import "../theme"
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
  // reactions are this message's reaction chips, or an empty list.
  readonly property var reactions: Timeline.reactions(root.message)

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
  // react asks the caller to toggle this message's reaction with emoji.
  signal react(string id, string emoji)
  // reactPickerRequested asks the caller to open the emoji picker for
  // this message, from the hover toolbar's "+" button.
  signal reactPickerRequested(string id)

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

      // A passive hover handler shows the hover toolbar without taking
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

        // The text, then any link preview, photo or file; parts a
        // message lacks take no space.
        MessageBubbleContent {
          id: content

          x: bubble.padding
          y: bubble.padding
          message: root.message
          media: root.media
          quote: root.quote
          maxTextWidth: bubble.maxTextWidth
          onMediaWanted: root.mediaWanted(root.message.id)
          onMediaOpen: root.mediaOpen(root.message.id)
          onQuoteOpened: remoteId => root.quoteOpened(remoteId)
        }

        MessageMeta {
          id: meta

          y: content.y + content.height + Theme.spacing.xxs
          anchors.right: root.message.outgoing ? parent.right : undefined
          anchors.left: root.message.outgoing ? undefined : parent.left
          anchors.leftMargin: bubble.padding
          anchors.rightMargin: bubble.padding
          message: root.message
          nowMs: root.nowMs
          showStatus: root.showStatus
        }

        // hoverToolbar floats over the bubble's top corner, mostly above
        // it with only a sliver overlapping the top padding, never the
        // text: the near-the-middle corner for each direction, so it
        // never lands over the window edge. It sits outside the message
        // Column entirely, so fading it in or out never resizes or moves
        // any bubble; z keeps it drawn above the bubble's own content.
        MessageHoverToolbar {
          id: hoverToolbar

          objectName: "hoverToolbar"
          z: 1
          active: rowHover.hovered || toolbarHover.hovered
          x: root.message.outgoing ? 0 : bubble.width - width
          y: -height + Theme.spacing.xs
          onReact: root.reactPickerRequested(root.message.id)
          onReply: root.replyRequested(root.message.id)

          HoverHandler { id: toolbarHover }
        }
      }
    }

    ReactionChips {
      objectName: "reactionChips"
      width: parent.width
      alignRight: root.message.outgoing
      reactions: root.reactions
      onToggled: emoji => root.react(root.message.id, emoji)
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
