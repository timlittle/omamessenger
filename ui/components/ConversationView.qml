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
  // highlightedId is the message id j/k move through, by id rather than
  // index, or "" when nothing is highlighted.
  property string highlightedId: ""
  // nowMs is the current time, passed to each message delegate.
  property real nowMs: 0
  // draft is the composer's saved text for this conversation.
  property string draft: ""
  // replyTarget is the message the composer is about to answer: {id,
  // senderName, text}, or null.
  property var replyTarget: null
  // attachmentPath is the file to send with the next message, or "" for
  // none; see Composer.qml for why the caller owns it.
  property string attachmentPath: ""
  // composeEnabled is false while the conversation can't accept input.
  property bool composeEnabled: true
  // composer exposes the Composer instance so a key router can focus it.
  property alias composer: composer
  // routeKey is forwarded straight to the composer; see Composer.qml for
  // why a key router intercepts through a function property, not a signal.
  property var routeKey: null
  // isGroup is true when the open conversation is a group chat.
  readonly property bool isGroup: root.conversation ? root.conversation.kind === "group" : false

  // loadOlder asks the caller to fetch messages before the oldest loaded
  // one. It fires on every scroll near the top, so the caller ignores it
  // while a page is loading or when there is nothing older.
  signal loadOlder()
  // retry asks the caller to resend a failed outgoing message.
  signal retry(string id)
  // mediaWanted asks for a message's photo to be downloaded.
  signal mediaWanted(string id)
  // mediaOpen asks for a message's photo, video or file to be opened.
  signal mediaOpen(string id)
  // react asks the caller to toggle a message's reaction with emoji.
  signal react(string id, string emoji)
  // reactPickerRequested asks the caller to open the emoji picker for a
  // message, from its chips row's "+" button.
  signal reactPickerRequested(string id)
  // send reports a message the user submitted, and the id of the message
  // it answers, or "" when it answers nothing.
  signal send(string text, string replyToId)
  // draftEdited reports the composer's text as the user types it.
  signal draftEdited(string text)
  // replyRequested asks the caller to start replying to a loaded message.
  signal replyRequested(string id)
  // replyCanceled asks the caller to clear the reply in progress.
  signal replyCanceled()
  // quoteOpened asks the caller to scroll to the message a reply quotes,
  // by the remote id the quote carries.
  signal quoteOpened(string remoteId)
  // fileAttached reports a file the attach button's own picker chose.
  signal fileAttached(string path)
  // attachmentRemoveRequested asks the caller to clear attachmentPath.
  signal attachmentRemoveRequested()

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

  // scrollToMessage brings a loaded message into view, by its local id.
  // Nothing happens when it is not loaded.
  function scrollToMessage(id: string): void {
    for (let i = 0; i < messageList.count; i++) {
      const row = messageList.model.get ? messageList.model.get(i) : messageList.model[i];
      if (row && row.id === id) {
        messageList.positionViewAtIndex(i, ListView.Contain);
        return;
      }
    }
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

  // replyTarget is copied into the composer the same way, so sending or
  // cancelling a reply inside the composer does not fight a live binding.
  onReplyTargetChanged: {
    if (composer.replyTo !== root.replyTarget)
      composer.replyTo = root.replyTarget;
  }

  Component.onCompleted: {
    composer.text = root.draft;
    composer.replyTo = root.replyTarget;
  }

  // _clampContentY keeps a scroll target within the list's scrollable range.
  // originY is not 0 for a BottomToTop ListView, and shifts as delegates
  // are created and their estimated sizes are replaced by real ones, so the
  // range has to track originY rather than assume the content starts at 0.
  function _clampContentY(y: real): real {
    const oldestY = messageList.originY
    const newestY = oldestY + Math.max(0, messageList.contentHeight - messageList.height)
    return Math.max(oldestY, Math.min(newestY, y))
  }

  // _checkLoadOlder emits loadOlder() once the view is within one viewport
  // of the oldest loaded message, which for a BottomToTop ListView sits at
  // originY, so that is the distance still to travel.
  function _checkLoadOlder(): void {
    if (messageList.contentHeight <= messageList.height) return
    const remaining = messageList.contentY - messageList.originY
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
          objectName: "conversationTitle"
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
      objectName: "messageListView"
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
        highlighted: root.highlightedId !== "" && modelData.id === root.highlightedId
        onRetry: id => root.retry(id)
        onMediaWanted: id => root.mediaWanted(id)
        onMediaOpen: id => root.mediaOpen(id)
        onReplyRequested: id => root.replyRequested(id)
        onQuoteOpened: remoteId => root.quoteOpened(remoteId)
        onReact: (id, emoji) => root.react(id, emoji)
        onReactPickerRequested: id => root.reactPickerRequested(id)
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
      attachmentPath: root.attachmentPath
      routeKey: root.routeKey
      onSubmitted: (text, replyToId) => root.send(text, replyToId)
      onTextChanged: root.draftEdited(composer.text)
      onReplyCanceled: root.replyCanceled()
      onFileAttached: path => root.fileAttached(path)
      onAttachmentRemoveRequested: root.attachmentRemoveRequested()
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
