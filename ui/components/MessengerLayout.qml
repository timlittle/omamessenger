import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Rail.js" as Rail

// The three columns that make up the OmaMessenger window: the service
// rail, the search field and conversation list, and the open
// conversation. This is Panel.qml's own body, split out only to keep that
// file within the size guideline: unlike the views under ui/components it
// is allowed to call the controllers directly, binding their data into
// the views below and their functions to the views' signals. Panel.qml
// keeps every service.request call and the keyboard router; the floating
// overlays (the palette, the dialogs, the photo viewer) live in
// Overlays.qml, this file's sibling.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null
  // listController is bound into the rail and the list.
  property var listController: null
  // conversationController is bound into the open conversation.
  property var conversationController: null
  // composerController is bound into the composer.
  property var composerController: null
  // photoViewerController is bound into the overlays' photo viewer.
  property var photoViewerController: null
  // reactionsController is bound into the message list and the overlays'
  // emoji picker.
  property var reactionsController: null
  // deleteController is bound into the message list and the overlays'
  // delete question.
  property var deleteController: null
  // dialogController is bound into the new-chat dialog.
  property var dialogController: null
  // accountController is bound into account setup and the empty state.
  property var accountController: null
  // windowController is bound into the shortcut help sheet.
  property var windowController: null
  // nowMs is the current time, refreshed by Panel.qml, for relative times.
  property real nowMs: Date.now()
  // routeKey is Panel's router: called with (key, modifiers, text) from
  // the search field, the composer and the dialog's search field, before
  // each handles its own key presses. See Composer.qml for why it is a
  // function property passed down, rather than a signal carrying the
  // KeyEvent back up.
  property var routeKey: null
  // focusDefault returns keyboard focus to Panel's own key area. Leaving
  // the search field or the composer only clears that field's `focus`,
  // which does not reliably hand active focus back to anything, so
  // Panel passes this down to call after each such blur.
  property var focusDefault: null

  // _railWidth is the rail's fixed width.
  readonly property real _railWidth: Style.space(64)
  // _listWidth clamps the list column between 260 and 360, scaled with
  // whatever width the rail leaves it.
  readonly property real _listWidth: Math.max(Style.space(260),
    Math.min(Style.space(360), 0.32 * (columns.width - root._railWidth)))

  // narrow is true when the list and an open conversation do not both fit
  // beside the rail. The window then shows one of them at a time,
  // following the conversation controller's pane.
  readonly property bool narrow: root.width < root._railWidth + Style.space(260) + Style.space(360)

  // _showList and _showConversation pick the columns a narrow window shows.
  readonly property bool _showList: !root.narrow || root.conversationController.pane !== "conversation"
  readonly property bool _showConversation: !root.narrow || root.conversationController.pane === "conversation"

  // _openRow opens the conversation a list row or a dialog contact chose.
  function _openRow(id: string): void {
    const conversation = root.listController.findConversation(id)
    if (conversation) root.conversationController.open(conversation)
  }

  // _scrollConversation turns a ConversationController.scroll() direction
  // into the matching ConversationView call.
  function _scrollConversation(direction: string): void {
    if (direction === "down") conversationView.scrollBy(1)
    else if (direction === "up") conversationView.scrollBy(-1)
    else if (direction === "pageDown") conversationView.scrollPage(1)
    else if (direction === "pageUp") conversationView.scrollPage(-1)
    else if (direction === "newest") conversationView.scrollToNewest()
    else if (direction === "oldest") conversationView.scrollToOldest()
  }

  Connections {
    target: root.conversationController

    function onActiveIdChanged() {
      if (root.conversationController.activeId) conversationView.focusComposer()
    }
    function onScroll(direction) { root._scrollConversation(direction) }
    function onScrollToMessageRequested(id) { conversationView.scrollToMessage(id) }
    function onLinksRequested(urls) {
      if (urls.length === 1) Qt.openUrlExternally(urls[0]);
      else root.windowController.openLinkChooser(urls);
    }
  }

  // Composer focus mirrors ComposerController.composeFocused both ways: a
  // request from the controller moves real focus, and real focus changes
  // (a click, or leaving) are read back into the controller.
  Connections {
    target: root.composerController

    function onComposeFocusRequested() { conversationView.focusComposer() }
    function onSubmitRequested() { conversationView.composer.submit() }
    function onNewlineRequested() {
      const input = conversationView.composer.input
      input.insert(input.cursorPosition, "\n")
    }
    function onLeaveComposeRequested() {
      conversationView.composer.input.focus = false
      if (root.focusDefault) root.focusDefault()
    }
    function onAttachFileRequested() { conversationView.composer.openFilePicker() }
    function onPasteFallbackRequested() { conversationView.composer.input.paste() }
  }

  Connections {
    target: conversationView.composer.input
    function onActiveFocusChanged() {
      root.composerController.composeFocused = conversationView.composer.input.activeFocus
    }
  }

  // Search focus mirrors ListController.searchFocused the same way: a
  // request focuses the field, a field focus change is read back, and the
  // field is blurred if the controller leaves search while it still holds
  // real focus (the Escape chain only updates the controller's flag).
  Connections {
    target: root.listController

    function onFocusRequested() { listColumn.searchField.forceActiveFocus() }
    function onSearchFocusedChanged() {
      if (root.listController.searchFocused || !listColumn.searchField.activeFocus) return
      listColumn.searchField.focus = false
      if (root.focusDefault) root.focusDefault()
    }
  }

  Connections {
    target: listColumn.searchField
    function onActiveFocusChanged() {
      root.listController.searchFocused = listColumn.searchField.activeFocus
    }
  }

  RowLayout {
    id: columns
    anchors.fill: parent
    spacing: Theme.spacing.sm

    ServiceRail {
      id: rail
      objectName: "serviceRail"
      Layout.preferredWidth: root._railWidth
      Layout.fillHeight: true

      items: root.listController.railItems
      selectedKey: root.listController.railKey
      knownServices: root.service ? root.service.services : []

      onSelected: key => root.listController.setRail(key)
      onNewChat: root.dialogController.run("chat.new")
      onHelp: root.windowController.run("palette.commands")
    }

    // Pinned to its width: a layout whose children fill would otherwise
    // fill the row too and squeeze the conversation off the edge. In a
    // narrow window it is the only column, so it fills.
    ListColumn {
      id: listColumn
      objectName: "listColumn"
      visible: root._showList
      Layout.fillWidth: root.narrow
      Layout.minimumWidth: root.narrow ? 0 : root._listWidth
      Layout.maximumWidth: root.narrow ? Number.POSITIVE_INFINITY : root._listWidth
      Layout.preferredWidth: root._listWidth
      Layout.fillHeight: true

      query: root.listController.query
      model: root.listController.model
      selectedId: root.listController.selectedId
      nowMs: root.nowMs
      accountNames: Rail.accountNames(root.service ? root.service.accounts : [])
      multiAccountServices: Rail.multiAccountServices(root.listController.railItems)
      showEmptyState: root.service !== null && root.service.status === "ready" && root.service.accounts.length === 0
      showAll: root.listController.showAll
      hiddenCount: root.listController.hiddenCount
      unreadView: root.listController.unreadView
      routeKey: root.routeKey

      onQueryEdited: text => root.listController.setQuery(text)
      onActivated: id => root._openRow(id)
      onAddAccountRequested: root.accountController.begin()
      onShowAllToggled: root.listController.setShowAll(!root.listController.showAll)
      onUnreadViewLeft: root.listController.setUnreadView(false)
    }

    ConversationView {
      id: conversationView
      objectName: "conversationView"
      visible: root._showConversation
      Layout.fillWidth: true
      Layout.fillHeight: true

      conversation: root.conversationController.conversation
      subtitle: root.conversationController.subtitle
      messages: root.conversationController.messages
      annotations: root.conversationController.annotations
      highlightedId: root.conversationController.highlightedId
      nowMs: root.nowMs
      draft: root.composerController.draft
      replyTarget: root.composerController.replyTarget
      attachmentPath: root.composerController.attachmentPath
      composeEnabled: root.conversationController.activeId !== ""
      voiceNotes: root.conversationController.voiceNotes
      members: root.conversationController.groupMembers
      routeKey: root.routeKey

      onLoadOlder: root.conversationController.loadOlder()
      onRetry: id => root.conversationController.retryMessage(id)
      onMediaWanted: id => root.conversationController.fetchMedia(id)
      onMediaOpen: id => root.conversationController.openMedia(id)
      onReact: (id, emoji) => root.reactionsController.react(id, emoji)
      onReactPickerRequested: id => root.reactionsController.openPicker(id)
      onDeleteRequested: id => { if (root.deleteController) root.deleteController.openConfirm(id) }
      onSend: (text, replyToId, mentions) => root.conversationController.send(text, replyToId, mentions)
      onDraftEdited: text => root.composerController.setDraft(text)
      onReplyRequested: id => root.composerController.startReply(id)
      onReplyCanceled: root.composerController.cancelReply()
      onQuoteOpened: remoteId => root.conversationController.scrollToReply(remoteId)
      onFileAttached: path => root.composerController.attachFile(path)
      onAttachmentRemoveRequested: root.composerController.removeAttachment()
    }
  }

  Overlays {
    anchors.fill: parent
    service: root.service
    windowController: root.windowController
    dialogController: root.dialogController
    accountController: root.accountController
    photoViewerController: root.photoViewerController
    reactionsController: root.reactionsController
    deleteController: root.deleteController
    routeKey: root.routeKey
    focusDefault: root.focusDefault
  }
}
