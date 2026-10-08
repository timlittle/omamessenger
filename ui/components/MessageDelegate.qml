import QtQuick
import qs.Commons
import "../theme"
import "../lib/Timeline.js" as Timeline
import "../lib/Highlight.js" as Highlight
import "../lib/SenderColor.js" as SenderColor
import "../lib/Keymap.js" as Keymap
import "../lib/Format.js" as Format
import "../lib/Media.js" as Media

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
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), read for the key-hint row below.
  property var bindings: Keymap.BINDINGS
  // nowMs is the current time, read for the failed-send retry line below.
  // Defaults to the shared clock; a test overrides it to pick an
  // arbitrary time.
  property real nowMs: Clock.nowMs
  // voiceNotes is the playback state the voice note player reads; see
  // MessageBubbleContent for its shape. Recycled delegates only ever
  // read this, never own it, so scrolling never interrupts playback.
  property var voiceNotes: Media.NO_VOICE_NOTE
  // pollVote is this message's vote-mode state, read from
  // PollsController, or the default while it is not the message voting:
  // {voting, highlightedIndex, selectedIds}. Recycled delegates only
  // ever read this, never own it, the same way voiceNotes works.
  property var pollVote: ({ voting: false, highlightedIndex: -1, selectedIds: [] })
  // highlighted is true when this is the message the keyboard currently
  // points at: shown with a thin accent outline and a short accent bar
  // on the bubble itself, and a row of key hints for what pressing a
  // key does to it. The caller hides this while the composer has focus,
  // so scrolling and writing never show it at the same time.
  property bool highlighted: false
  // failed is true for an outgoing message the service could not send.
  readonly property bool failed: root.message.outgoing && root.message.status === "failed"
  // retryAt is the helper's next scheduled automatic retry for a failed
  // outgoing message (Unix milliseconds), or 0 while none is scheduled,
  // either because the failure was permanent or automatic retries have
  // run for as long as they will.
  readonly property real retryAt: root.message.retryAt ?? 0
  // showStatus is true when the delivery glyph should be drawn.
  readonly property bool showStatus: root.message.outgoing && !root.failed
  // media is what the message carries besides its text, or null.
  readonly property var media: Timeline.media(root.message)
  // isSticker is true for a sticker message, shown with no bubble
  // behind it at all, unlike every other media kind.
  readonly property bool isSticker: !!root.media && root.media.kind === "sticker"
  // quote is the message this one replies to, or null.
  readonly property var quote: Timeline.replyTo(root.message)
  // reactions are this message's reaction chips, or an empty list.
  readonly property var reactions: Timeline.reactions(root.message)
  // mentions are this message's @-mention tokens, or an empty list.
  readonly property var mentions: Timeline.mentions(root.message)
  // nameColor is this message's sender's colour in a group chat, stable
  // for the same sender across every message; direct chats keep the
  // plain foreground colour the name used before group colouring existed.
  readonly property color nameColor: root.isGroup
    ? Theme.senderColor(SenderColor.colorIndex(root.message.senderId, Theme.senderPaletteSize))
    : Color.foreground
  // showAvatar is true only for the first message of a run from one
  // sender in a group chat: the message the avatar and name sit beside.
  readonly property bool showAvatar: root.isGroup && !root.message.outgoing && root.annotation.showSender
  // groupIndent is true for every incoming message in a group chat, so a
  // run's later bubbles reserve the same left margin as the first one
  // even though only the first shows an avatar there.
  readonly property bool groupIndent: root.isGroup && !root.message.outgoing
  // avatarSize and avatarGap size the reserved column; indentWidth is the
  // total space it takes out of the row, zero outside a group chat.
  readonly property real avatarSize: Style.space(28)
  readonly property real avatarGap: Theme.spacing.sm
  readonly property real indentWidth: root.groupIndent ? root.avatarSize + root.avatarGap : 0

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
  // deleteRequested asks the caller to open the delete question for
  // this message, from the hover toolbar's delete button.
  signal deleteRequested(string id)
  // voted asks the caller to cast optionIds as the user's vote in this
  // message's poll.
  signal voted(string id, var optionIds)

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
      objectName: "senderName"
      x: Theme.spacing.md + root.indentWidth
      visible: root.annotation.showSender
      text: root.message.senderName
      color: root.nameColor
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

      // The run's first incoming message in a group chat gets an avatar
      // in its sender's colour, vertically centred on the bubble; later
      // messages in the run show none, but still reserve the same space
      // through the bubble's own left margin below, so every bubble in
      // the run lines up under the first one.
      Avatar {
        objectName: "senderAvatar"
        visible: root.showAvatar
        name: root.message.senderName
        size: root.avatarSize
        tint: root.nameColor
        tinted: true
        anchors.left: parent.left
        anchors.verticalCenter: bubble.verticalCenter
      }

      // The bubble wraps its text: as wide as the text needs, up to 72% of
      // the row, after which the text wraps.
      Rectangle {
        id: bubble

        // A sticker shows with no bubble behind it at all: no padding,
        // no fill, just the image itself, still keyboard-highlightable
        // through the same outline every other message gets.
        readonly property real padding: root.isSticker ? 0 : Theme.spacing.sm
        readonly property real maxTextWidth: (root.width - root.indentWidth) * 0.72 - padding * 2

        objectName: "bubble"
        width: Math.max(content.width, meta.implicitWidth) + padding * 2
        height: meta.y + meta.implicitHeight + padding
        radius: Style.cornerRadius
        // mentionsMe tints the bubble a little more strongly than usual,
        // paired below with an "@" glyph: colour alone must never be the
        // only cue (see qml.md).
        color: root.isSticker ? "transparent" : (root.message.outgoing
          ? Util.alpha(Color.accent, root.message.mentionsMe ? 0.32 : 0.22)
          : (root.message.mentionsMe ? Util.alpha(Color.accent, 0.16) : Util.alpha(Color.foreground, 0.06)))
        anchors.right: root.message.outgoing ? parent.right : undefined
        anchors.left: root.message.outgoing ? undefined : parent.left
        anchors.leftMargin: root.message.outgoing ? 0 : root.indentWidth
        // The highlighted message's own cue: a thin accent outline, no
        // wider than the bubble itself, so nothing beyond it ever tints.
        // A message that mentions the signed-in user keeps a fainter
        // version of the same outline even while not highlighted.
        border.width: root.highlighted || root.message.mentionsMe ? Style.space(1) : 0
        border.color: root.highlighted ? Color.accent : (root.message.mentionsMe ? Util.alpha(Color.accent, 0.5) : "transparent")

        // The glyph that pairs with the tint above: an own-identity cue
        // text alone (colour) would fail someone who cannot see it.
        Text {
          objectName: "mentionGlyph"
          visible: !!root.message.mentionsMe
          text: "@"
          color: Color.accent
          anchors.top: parent.top
          anchors.right: root.message.outgoing ? parent.right : undefined
          anchors.left: root.message.outgoing ? undefined : parent.left
          anchors.margins: Theme.spacing.xxs
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
          font.weight: Font.DemiBold
        }

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
          mentions: root.mentions
          maxTextWidth: bubble.maxTextWidth
          voiceNotes: root.voiceNotes
          pollVote: root.pollVote
          onMediaWanted: root.mediaWanted(root.message.id)
          onMediaOpen: root.mediaOpen(root.message.id)
          onQuoteOpened: remoteId => root.quoteOpened(remoteId)
          onVoted: optionIds => root.voted(root.message.id, optionIds)
        }

        MessageMeta {
          id: meta

          y: content.y + content.height + Theme.spacing.xxs
          anchors.right: root.message.outgoing ? parent.right : undefined
          anchors.left: root.message.outgoing ? undefined : parent.left
          anchors.leftMargin: bubble.padding
          anchors.rightMargin: bubble.padding
          message: root.message
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
          // case it is not. The group-chat indent eats into this space on
          // an incoming bubble's right side exactly as much as it shifts
          // the bubble's left edge, so it has to come off the total too.
          readonly property real besideSpace: bubbleRow.width - bubble.width - root.indentWidth
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
          onDeleteRequested: root.deleteRequested(root.message.id)

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
      text: Highlight.hints(root.message, root.bindings)
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

    // The failed-send line: "Not sent" always, paired either with the
    // "t to retry" hint while no automatic retry is scheduled, or with
    // a quiet, nowMs-driven "Retrying …" once one is (see Format.js).
    // Manual retry still works either way: clicking anywhere on the
    // line resets the backoff, same as pressing t.
    Item {
      width: parent.width
      height: retryRow.height
      visible: root.failed

      Row {
        id: retryRow
        anchors.right: parent.right
        spacing: Theme.spacing.xxs

        Text {
          text: "Not sent"
          color: Color.urgent
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
        }

        Text {
          visible: root.retryAt === 0
          text: "· t to retry"
          color: Color.urgent
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
        }

        Text {
          objectName: "retryingText"
          visible: root.retryAt !== 0
          text: Format.retryingLabel(root.retryAt, root.nowMs)
          color: Util.alpha(Color.foreground, 0.5)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.caption
        }
      }

      MouseArea {
        anchors.fill: retryRow
        cursorShape: Qt.PointingHandCursor
        onClicked: root.retry(root.message.id)
      }
    }
  }
}
