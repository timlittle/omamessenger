pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Keymap.js" as Keymap
import "../lib/Media.js" as Media

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
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), passed to each message delegate's
  // hint row and to the composer's mode hint.
  property var bindings: Keymap.BINDINGS
  // voiceNotes is the playback state each message delegate reads for its
  // voice note player; see MessageBubbleContent for its shape.
  property var voiceNotes: Media.NO_VOICE_NOTE
  // videoPlayback is the playback state each message delegate reads for
  // its video player; see MessageBubbleContent for its shape.
  property var videoPlayback: ({ playingId: "", muted: true, available: true })
  // windowActive is whether the user can see the window at all: shown
  // and focused, read by each video player to pause while it is false.
  property bool windowActive: true
  // voteState is PollsController's vote mode, read by whichever message
  // delegate's id matches target: {target, highlightedIndex, selectedIds}.
  property var voteState: ({ target: "", highlightedIndex: -1, selectedIds: [] })
  // draft is the composer's saved text for this conversation.
  property string draft: ""
  // replyTarget is the message the composer is about to answer: {id,
  // senderName, text}, or null.
  property var replyTarget: null
  // editTarget is the message the composer is about to save new text
  // for: {id}, or null while not editing.
  property var editTarget: null
  // attachmentPath is the file to send with the next message, or "" for
  // none; see Composer.qml for why the caller owns it.
  property string attachmentPath: ""
  // composeEnabled is false while the conversation can't accept input.
  property bool composeEnabled: true
  // historyUnavailable is true once scrolling back asked the service for
  // older history and it could not be reached right now; shown as a
  // small note above the oldest loaded message rather than silently
  // stopping, so the user knows to try again once their phone is online.
  property bool historyUnavailable: false
  // composer exposes the Composer instance so a key router can focus it.
  property alias composer: composer
  // routeKey is forwarded straight to the composer; see Composer.qml for
  // why a key router intercepts through a function property, not a signal.
  property var routeKey: null
  // isGroup is true when the open conversation is a group chat.
  readonly property bool isGroup: root.conversation ? root.conversation.kind === "group" : false
  // _blocked is true while the open conversation's contact is blocked,
  // which disables the composer and shows why (see Composer.blocked).
  readonly property bool _blocked: root.conversation ? !!root.conversation.blocked : false
  // members are the open group's members, for the composer's @-mention
  // picker; an empty list for a direct chat.
  property var members: []
  // misspellings are the composer's last spell.check result:
  // [{start, end, word}] in UTF-16 offsets. Empty while spell check is
  // off or nothing is flagged.
  property var misspellings: []
  // suggestions are the last spell.suggest result, for whichever word
  // the spelling picker most recently asked about.
  property var suggestions: []

  // loadOlder asks the caller to fetch messages before the oldest loaded
  // one. It fires on every scroll near the top, so the caller ignores it
  // while a page is loading or when there is nothing older.
  signal loadOlder()
  // retry asks the caller to resend a failed outgoing message.
  signal retry(string id)
  // mediaWanted asks for a message's photo to be downloaded.
  signal mediaWanted(string id)
  // mediaRetry asks for a message's photo, sticker or voice note to be
  // downloaded deliberately, the tap-to-load action offered once
  // mediaWanted's own automatic fetch was refused only for being too
  // large.
  signal mediaRetry(string id)
  // mediaOpen asks for a message's photo, video or file to be opened.
  signal mediaOpen(string id)
  // react asks the caller to toggle a message's reaction with emoji.
  signal react(string id, string emoji)
  // reactPickerRequested asks the caller to open the emoji picker for a
  // message, from its chips row's "+" button.
  signal reactPickerRequested(string id)
  // deleteRequested asks the caller to open the delete question for a
  // message, from its hover toolbar's delete button.
  signal deleteRequested(string id)
  // videoToggled asks the caller to select, deselect or pause a
  // message's video.
  signal videoToggled(string id)
  // videoMuteToggled asks the caller to flip the shared mute preference
  // every video reads.
  signal videoMuteToggled(string id)
  // mediaCopyRequested asks the caller to copy a message's photo or
  // video to the clipboard.
  signal mediaCopyRequested(string id)
  // mediaSaveRequested asks the caller to save a message's photo or
  // video into the downloads directory.
  signal mediaSaveRequested(string id)
  // mediaOpenExternalRequested asks the caller to open a message's
  // photo or video in the user's own application.
  signal mediaOpenExternalRequested(string id)
  // voted asks the caller to cast optionIds as the user's vote in a
  // message's poll.
  signal voted(string id, var optionIds)
  // linkOpenRequested asks the caller to open url: a link in a
  // message's own text, or its link preview's URL.
  signal linkOpenRequested(string url)
  // send reports a message the user submitted, the id of the message it
  // answers (or "" when it answers nothing), and its resolved @-mentions.
  signal send(string text, string replyToId, var mentions)
  // draftEdited reports the composer's text as the user types it.
  signal draftEdited(string text)
  // replyRequested asks the caller to start replying to a loaded message.
  signal replyRequested(string id)
  // replyCanceled asks the caller to clear the reply in progress.
  signal replyCanceled()
  // editCanceled asks the caller to clear the edit in progress.
  signal editCanceled()
  // quoteOpened asks the caller to scroll to the message a reply quotes,
  // by the remote id the quote carries.
  signal quoteOpened(string remoteId)
  // fileAttached reports a file the attach button's own picker chose.
  signal fileAttached(string path)
  // attachmentRemoveRequested asks the caller to clear attachmentPath.
  signal attachmentRemoveRequested()
  // spellSuggestionsRequested asks the caller to fetch replacement words
  // for word, for the spelling picker the composer just opened.
  signal spellSuggestionsRequested(string word)
  // spellWordAdded asks the caller to add word to the personal
  // dictionary.
  signal spellWordAdded(string word)

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

  // editTarget is copied into the composer the same way replyTarget is.
  onEditTargetChanged: {
    if (composer.editTarget !== root.editTarget)
      composer.editTarget = root.editTarget;
  }

  Component.onCompleted: {
    composer.text = root.draft;
    composer.replyTo = root.replyTarget;
    composer.editTarget = root.editTarget;
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
          // A contact or group title is sender-controlled; PlainText
          // keeps it from ever being auto-detected as rich text.
          textFormat: Text.PlainText
          elide: Text.ElideRight
          color: Theme.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.title
          font.weight: Font.DemiBold
        }

        Text {
          Layout.fillWidth: true
          text: root.subtitle
          textFormat: Text.PlainText
          elide: Text.ElideRight
          color: Util.alpha(Theme.foreground, 0.6)
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
        bindings: root.bindings
        voiceNotes: root.voiceNotes
        videoPlayback: root.videoPlayback
        windowActive: root.windowActive
        pollVote: ({
          voting: root.voteState.target !== "" && root.voteState.target === modelData.id,
          highlightedIndex: root.voteState.highlightedIndex,
          selectedIds: root.voteState.selectedIds
        })
        // The highlight and its hint row only make sense in scroll
        // mode: while the composer has focus, j/k do not move it and
        // r/e/t/Enter do not act on it, so showing it would say
        // something no longer true.
        highlighted: !composer.writing && root.highlightedId !== "" && modelData.id === root.highlightedId
        onRetry: id => root.retry(id)
        onMediaWanted: id => root.mediaWanted(id)
        onMediaRetry: id => root.mediaRetry(id)
        onMediaOpen: id => root.mediaOpen(id)
        onReplyRequested: id => root.replyRequested(id)
        onQuoteOpened: remoteId => root.quoteOpened(remoteId)
        onReact: (id, emoji) => root.react(id, emoji)
        onReactPickerRequested: id => root.reactPickerRequested(id)
        onDeleteRequested: id => root.deleteRequested(id)
        onVoted: (id, optionIds) => root.voted(id, optionIds)
        onVideoToggled: id => root.videoToggled(id)
        onVideoMuteToggled: id => root.videoMuteToggled(id)
        onMediaCopyRequested: id => root.mediaCopyRequested(id)
        onMediaSaveRequested: id => root.mediaSaveRequested(id)
        onMediaOpenExternalRequested: id => root.mediaOpenExternalRequested(id)
        onLinkOpenRequested: url => root.linkOpenRequested(url)
      }

      onContentYChanged: root._checkLoadOlder()
      onContentHeightChanged: root._checkLoadOlder()

      // A BottomToTop ListView's footer sits at the layout's end, which
      // for this direction is the visual top: right above the oldest
      // loaded message, where scrolling back stopped.
      footer: Text {
        width: messageList.width
        visible: root.historyUnavailable
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.WordWrap
        topPadding: Theme.spacing.sm
        bottomPadding: Theme.spacing.sm
        text: "Older messages need your phone online — try again later"
        textFormat: Text.PlainText
        color: Util.alpha(Theme.foreground, 0.6)
        font.family: Theme.font.family
        font.pixelSize: Theme.font.bodySmall
      }
    }

    Composer {
      id: composer
      Layout.fillWidth: true
      Layout.margins: Theme.spacing.md
      // active is false, destroying the text input, whenever there is
      // nothing to compose into at all: no conversation open, or this
      // whole view hidden (a narrow window showing the list instead;
      // see MessengerLayout.qml's own _showConversation, which sets
      // root.visible). See Composer.qml's own active for why.
      active: root.visible && root.conversation !== null
      enabled: root.composeEnabled && !root._blocked
      blocked: root._blocked
      attachmentPath: root.attachmentPath
      bindings: root.bindings
      routeKey: root.routeKey
      members: root.members
      misspellings: root.misspellings
      suggestions: root.suggestions
      onSubmitted: (text, replyToId, mentions) => root.send(text, replyToId, mentions)
      onTextChanged: root.draftEdited(composer.text)
      onReplyCanceled: root.replyCanceled()
      onEditCanceled: root.editCanceled()
      onFileAttached: path => root.fileAttached(path)
      onAttachmentRemoveRequested: root.attachmentRemoveRequested()
      onSpellSuggestionsRequested: word => root.spellSuggestionsRequested(word)
      onSpellWordAdded: word => root.spellWordAdded(word)
    }
  }

  Text {
    anchors.centerIn: parent
    visible: root.conversation === null
    text: "Pick a chat · j/k to move · Enter to open"
    textFormat: Text.PlainText
    color: Util.alpha(Theme.foreground, 0.5)
    font.family: Theme.font.family
    font.pixelSize: Theme.font.body
  }
}
