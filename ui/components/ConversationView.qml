pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"

// The open conversation: header, message list and composer. Shows an empty
// state instead when no conversation is open.
Item {
  id: root

  // conversation is the open Conversation, or null when nothing is open.
  property var conversation: null
  // subtitle is the line under the title, computed by the caller (members,
  // typing, or account status).
  property string subtitle: ""
  // messages is the loaded timeline, newest first.
  property var messages: []
  // annotations is Timeline.annotate's output, same order as messages.
  property var annotations: []
  // nowMs is the current time, passed to each message delegate.
  property real nowMs: 0
  // draft is the composer's saved text for this conversation.
  property string draft: ""
  // composeEnabled is false while the conversation can't accept input.
  property bool composeEnabled: true
  // composer exposes the Composer instance so a key router can focus it.
  property alias composer: composer
  // isGroup is true when the open conversation is a group chat.
  readonly property bool isGroup: root.conversation ? root.conversation.kind === "group" : false

  // loadOlder asks the caller to fetch messages before the oldest loaded
  // one. It fires on every scroll near the top, so the caller ignores it
  // while a page is loading or when there is nothing older.
  signal loadOlder()
  // retry asks the caller to resend a failed outgoing message.
  signal retry(string id)
  // send reports a message the user submitted.
  signal send(string text)
  // draftEdited reports the composer's text as the user types it.
  signal draftEdited(string text)

  // scrollBy moves the view by a number of lines; negative scrolls up.
  function scrollBy(lines: int): void {
    const step = Theme.font.body * 1.5
    messageList.contentY = root._clampContentY(messageList.contentY + lines * step)
  }

  // scrollPage moves the view by one viewport; direction is +1 down or -1 up.
  function scrollPage(direction: int): void {
    messageList.contentY = root._clampContentY(messageList.contentY + direction * messageList.height)
  }

  // scrollToNewest jumps to the newest loaded message.
  function scrollToNewest(): void {
    messageList.positionViewAtBeginning()
  }

  // scrollToOldest jumps to the oldest loaded message, which may trigger
  // loadOlder() if that leaves the view within one viewport of the end.
  function scrollToOldest(): void {
    messageList.positionViewAtEnd()
  }

  // focusComposer moves keyboard focus into the composer's text input.
  function focusComposer(): void {
    composer.focusInput()
  }

  // A binding from draft to the composer would break the first time the
  // composer clears itself after sending, so the draft is copied in on
  // every change instead.
  onDraftChanged: {
    if (composer.text !== root.draft)
      composer.text = root.draft;
  }

  Component.onCompleted: composer.text = root.draft

  // _clampContentY keeps a scroll target within the list's scrollable range.
  // A BottomToTop ListView keeps 0 at the newest message and grows negative
  // toward the oldest, so the valid range sits at or below zero.
  function _clampContentY(y: real): real {
    const oldestY = Math.min(0, -(messageList.contentHeight - messageList.height))
    return Math.max(oldestY, Math.min(0, y))
  }

  // _checkLoadOlder emits loadOlder() once the view is within one viewport
  // of the oldest loaded message. A BottomToTop ListView reaches the oldest
  // message at contentY = height - contentHeight, so that is the distance
  // still to travel.
  function _checkLoadOlder(): void {
    if (messageList.contentHeight <= messageList.height) return
    const remaining = messageList.contentY - (messageList.height - messageList.contentHeight)
    if (remaining <= messageList.height) root.loadOlder()
  }

  ColumnLayout {
    anchors.fill: parent
    visible: root.conversation !== null
    spacing: 0

    RowLayout {
      Layout.fillWidth: true
      Layout.margins: Theme.spacing.md
      spacing: Theme.spacing.controlGap

      Avatar {
        name: root.conversation ? root.conversation.title : ""
      }

      ColumnLayout {
        Layout.fillWidth: true
        spacing: 0

        Text {
          Layout.fillWidth: true
          text: root.conversation ? root.conversation.title : ""
          elide: Text.ElideRight
          color: Color.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.title
          font.weight: Font.DemiBold
        }

        Text {
          Layout.fillWidth: true
          text: root.subtitle
          elide: Text.ElideRight
          color: Util.alpha(Color.foreground, 0.6)
          font.family: Theme.font.family
          font.pixelSize: Theme.font.bodySmall
        }
      }
    }

    ListView {
      id: messageList
      Layout.fillWidth: true
      Layout.fillHeight: true
      clip: true
      verticalLayoutDirection: ListView.BottomToTop
      model: root.messages

      delegate: MessageDelegate {
        required property var modelData
        required property int index

        message: modelData
        annotation: root.annotations[index] ?? ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })
        isGroup: root.isGroup
        nowMs: root.nowMs
        onRetry: id => root.retry(id)
      }

      onContentYChanged: root._checkLoadOlder()
      onContentHeightChanged: root._checkLoadOlder()
    }

    Composer {
      id: composer
      Layout.fillWidth: true
      Layout.margins: Theme.spacing.md
      title: root.conversation ? root.conversation.title : ""
      enabled: root.composeEnabled
      onSubmitted: text => root.send(text)
      onTextChanged: root.draftEdited(composer.text)
    }
  }

  Text {
    anchors.centerIn: parent
    visible: root.conversation === null
    text: "Pick a chat · j/k to move · Enter to open"
    color: Util.alpha(Color.foreground, 0.5)
    font.family: Theme.font.family
    font.pixelSize: Theme.font.body
  }
}
