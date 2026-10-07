import QtQuick
import qs.Commons
import "../theme"
import "../lib/Timeline.js" as Timeline
import "../lib/Highlight.js" as Highlight

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
  // voiceNotes is the playback state the voice note player reads; see
  // MessageBubbleContent for its shape. Recycled delegates only ever
  // read this, never own it, so scrolling never interrupts playback.
  property var voiceNotes: ({ available: false, playingId: "", positionMs: 0, durationMs: 0 })
  // highlighted is true when this is the message the keyboard currently
  // points at: shown with a thin accent outline and a short accent bar
  // on the bubble itself, and a row of key hints for what pressing a
  // key does to it. The caller hides this while the composer has focus,
  // so scrolling and writing never show it at the same time.
  property bool highlighted: false
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
        // The highlighted message's own cue: a thin accent outline, no
        // wider than the bubble itself, so nothing beyond it ever tints.
        border.width: root.highlighted ? Style.space(1) : 0
        border.color: root.highlighted ? Color.accent : "transparent"

        // highlightMark pairs the outline with a shape, not colour
        // alone: a short accent bar beside the bubble's own near edge,
        // exactly as tall as the bubble, never the whole row.
        Rectangle {
          objectName: "highlightBar"
          visible: root.highlighted
          anchors.top: parent.top
          anchors.bottom: parent.bottom
          anchors.topMargin: Theme.spacing.xxs
          anchors.bottomMargin: Theme.spacing.xxs
          anchors.left: root.message.outgoing ? undefined : parent.left
          anchors.right: root.message.outgoing ? parent.right : undefined
          anchors.leftMargin: root.message.outgoing ? 0 : Theme.spacing.xxs
          anchors.rightMargin: root.message.outgoing ? Theme.spacing.xxs : 0
          width: Style.space(2)
          radius: width / 2
          color: Color.accent
        }

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
          voiceNotes: root.voiceNotes
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

        // hoverToolbar sits beside the bubble, vertically centred on it,
        // in whatever free horizontal space the row leaves: to the left
        // of an outgoing bubble, to the right of an incoming one. That
        // space is never a neighbouring message's, so the toolbar can
        // never overlap one; clamping its y to the bubble's own height
        // keeps it there even when the bubble is shorter than the
        // toolbar. When the bubble is too wide to leave that room (an
        // unusually narrow window, or one at its maximum width), it
        // falls back inside the bubble's own top corner instead: still
        // this message's own space, never a neighbour's. It sits outside
        // the message Column entirely, so fading it in or out never
        // resizes or moves any bubble; z keeps it drawn above the
        // bubble's own content.
        MessageHoverToolbar {
          id: hoverToolbar

          // besideSpace is the free width beside the bubble: at least
          // 28% of the row, since the bubble's own width is capped at
          // 72% of it, which is enough in practice for the toolbar even
          // at the window's minimum size. fitsBeside guards the rare
          // case it is not.
          readonly property real besideSpace: bubbleRow.width - bubble.width
          readonly property bool fitsBeside: hoverToolbar.besideSpace >= hoverToolbar.width + Theme.spacing.xs

          objectName: "hoverToolbar"
          z: 1
          active: rowHover.hovered || toolbarHover.hovered
          x: hoverToolbar.fitsBeside
            ? (root.message.outgoing ? -(hoverToolbar.width + Theme.spacing.xs) : bubble.width + Theme.spacing.xs)
            : (root.message.outgoing ? Theme.spacing.xs : bubble.width - hoverToolbar.width - Theme.spacing.xs)
          y: hoverToolbar.fitsBeside
            ? Math.max(0, Math.min(bubble.height - hoverToolbar.height, (bubble.height - hoverToolbar.height) / 2))
            : Theme.spacing.xs
          onReact: root.reactPickerRequested(root.message.id)
          onReply: root.replyRequested(root.message.id)

          HoverHandler { id: toolbarHover }
        }
      }
    }

    // highlightHints always takes its row's space in the Column, so
    // showing or hiding it on the highlighted message never changes
    // this delegate's height and never shifts any other message; only
    // its opacity follows root.highlighted. The small left/right inset
    // keeps its first character from touching the ListView's own clip
    // edge, which otherwise clips a hair off it.
    Text {
      objectName: "highlightHints"
      opacity: root.highlighted ? 1 : 0
      anchors.right: root.message.outgoing ? parent.right : undefined
      anchors.left: root.message.outgoing ? undefined : parent.left
      anchors.rightMargin: root.message.outgoing ? Theme.spacing.xxs : 0
      anchors.leftMargin: root.message.outgoing ? 0 : Theme.spacing.xxs
      text: Highlight.hints(root.message)
      color: Util.alpha(Color.foreground, 0.5)
      font.family: Theme.font.family
      font.pixelSize: Theme.font.bodySmall
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
        text: "Not sent · t to retry"
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
