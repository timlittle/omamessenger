import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Rail.js" as Rail

// The three columns and the overlays that make up the OmaMessenger window:
// the service rail, the search field and conversation list, and the open
// conversation, plus the shortcut help sheet and the new-chat dialog. This
// is Panel.qml's own body, split out only to keep that file within the
// size guideline: unlike the views under ui/components it is allowed to
// call the controllers directly, binding their data into the views below
// and their functions to the views' signals. Panel.qml keeps every
// service.request call and the keyboard router; this file only wires.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null
  // listController is bound into the rail and the list.
  property var listController: null
  // conversationController is bound into the open conversation.
  property var conversationController: null
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

  // Composer focus mirrors ConversationController.composeFocused both
  // ways: a request from the controller moves real focus, and real focus
  // changes (a click, or leaving) are read back into the controller.
  Connections {
    target: root.conversationController

    function onActiveIdChanged() {
      if (root.conversationController.activeId) conversationView.focusComposer()
    }

    function onComposeFocusRequested() { conversationView.focusComposer() }
    function onSubmitRequested() { conversationView.composer.submit() }
    function onLeaveComposeRequested() {
      conversationView.composer.input.focus = false
      if (root.focusDefault) root.focusDefault()
    }
    function onScroll(direction) { root._scrollConversation(direction) }
  }

  Connections {
    target: conversationView.composer.input
    function onActiveFocusChanged() {
      root.conversationController.composeFocused = conversationView.composer.input.activeFocus
    }
  }

  // Search focus mirrors ListController.searchFocused the same way: a
  // request focuses the field, a field focus change is read back, and the
  // field is blurred if the controller leaves search while it still holds
  // real focus (the Escape chain only updates the controller's flag).
  Connections {
    target: root.listController

    function onFocusRequested() { searchField.forceActiveFocus() }
    function onSearchFocusedChanged() {
      if (root.listController.searchFocused || !searchField.activeFocus) return
      searchField.focus = false
      if (root.focusDefault) root.focusDefault()
    }
  }

  Connections {
    target: searchField
    function onActiveFocusChanged() {
      root.listController.searchFocused = searchField.activeFocus
    }
  }

  // Opening the new-chat dialog focuses its search field.
  Connections {
    target: root.dialogController
    function onOpenChanged() {
      if (root.dialogController.open) dialog.focusSearch()
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

      onSelected: key => root.listController.setRail(key)
      onNewChat: root.dialogController.run("chat.new")
      onHelp: root.windowController.run("palette.commands")
    }

    // Pinned to its width: a layout whose children fill would otherwise
    // fill the row too and squeeze the conversation off the edge. In a
    // narrow window it is the only column, so it fills.
    ColumnLayout {
      objectName: "listColumn"
      visible: root._showList
      Layout.fillWidth: root.narrow
      Layout.minimumWidth: root.narrow ? 0 : root._listWidth
      Layout.maximumWidth: root.narrow ? Number.POSITIVE_INFINITY : root._listWidth
      Layout.preferredWidth: root._listWidth
      Layout.fillHeight: true
      spacing: Theme.spacing.sm

      Ui.TextField {
        id: searchField
        Layout.fillWidth: true
        placeholderText: "Search"
        text: root.listController.query

        Keys.priority: Keys.BeforeItem
        Keys.onPressed: event => {
          if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
        }

        onTextChanged: root.listController.setQuery(text)
      }

      // With no accounts yet, the list says how to add one.
      ColumnLayout {
        objectName: "noAccounts"
        Layout.fillWidth: true
        visible: root.service !== null && root.service.status === "ready" && root.service.accounts.length === 0
        spacing: Theme.spacing.sm

        Text {
          Layout.fillWidth: true
          text: "No accounts yet. Add your Telegram account to see your chats here."
          wrapMode: Text.WordWrap
          color: Util.alpha(Color.foreground, 0.7)
          font { family: Theme.font.family; pixelSize: Theme.font.body }
        }

        Ui.Button {
          objectName: "addAccountButton"
          text: "Add Telegram account"
          focusable: true
          onClicked: root.accountController.begin()
        }
      }

      ConversationList {
        id: list
        Layout.fillWidth: true
        Layout.fillHeight: true

        model: root.listController.model
        selectedId: root.listController.selectedId
        query: root.listController.query
        nowMs: root.nowMs
        accountNames: Rail.accountNames(root.service ? root.service.accounts : [])
        multiAccountServices: Rail.multiAccountServices(root.listController.railItems)

        onActivated: id => root._openRow(id)
      }

      // Chats older than a month are hidden until asked for.
      RowLayout {
        objectName: "olderChats"
        Layout.fillWidth: true
        visible: root.listController.hiddenCount > 0 || (root.listController.showOlder && root.listController.query === "")
        spacing: Theme.spacing.sm

        Text {
          Layout.fillWidth: true
          text: root.listController.showOlder ? "Showing chats older than a month"
            : root.listController.hiddenCount + (root.listController.hiddenCount === 1 ? " older chat hidden" : " older chats hidden")
          elide: Text.ElideRight
          color: Util.alpha(Color.foreground, 0.7)
          font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
        }

        Ui.Button {
          objectName: "olderChatsButton"
          text: root.listController.showOlder ? "Hide" : "Show"
          focusable: true
          onClicked: root.listController.setShowOlder(!root.listController.showOlder)
        }
      }
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
      nowMs: root.nowMs
      draft: root.conversationController.draft
      composeEnabled: root.conversationController.activeId !== ""
      routeKey: root.routeKey

      onLoadOlder: root.conversationController.loadOlder()
      onRetry: id => root.conversationController.retryMessage(id)
      onMediaWanted: id => root.conversationController.fetchMedia(id)
      onMediaOpen: id => root.conversationController.openMedia(id)
      onSend: text => root.conversationController.send(text)
      onDraftEdited: text => root.conversationController.setDraft(text)
    }
  }

  CommandPalette {
    open: root.windowController.paletteOpen
    placeholder: root.windowController.paletteMode === "conversations" ? "Jump to a conversation" : "Type a command"
    items: root.windowController.paletteItems
    currentIndex: root.windowController.paletteIndex
    routeKey: root.routeKey
    onQueryEdited: text => root.windowController.setPaletteQuery(text)
    onAccepted: index => root.windowController.acceptPalette(index)
    onCancelled: root.windowController.closePalette()
    onOpenChanged: if (!open && root.focusDefault) root.focusDefault()
  }

  CloseConfirm {
    open: root.windowController.confirmingClose
    onKeep: root.windowController.keepInBackground()
    onQuit: root.windowController.quit()
    onCancelled: {
      root.windowController.cancelClose();
      if (root.focusDefault) root.focusDefault();
    }
  }

  // Tests may build the layout without an account controller.
  AccountSetup {
    visible: root.accountController?.open ?? false

    stage: root.accountController?.stage ?? "credentials"
    qr: root.accountController?.qr ?? ""
    hint: root.accountController?.hint ?? ""
    error: root.accountController?.lastError ?? ""
    busy: root.accountController?.busy ?? false
    routeKey: root.routeKey

    onCredentialsSubmitted: (apiId, apiHash) => root.accountController.submitCredentials(apiId, apiHash)
    onPhoneRequested: root.accountController.usePhone()
    onAnswered: value => root.accountController.answer(value)
    onCancelled: {
      root.accountController.cancel();
      if (root.focusDefault) root.focusDefault();
    }
  }

  RemoveAccount {
    open: root.accountController?.removing ?? false
    accounts: root.service ? root.service.accounts : []
    error: root.accountController?.lastError ?? ""

    onChosen: accountId => root.accountController.remove(accountId)
    onCancelled: {
      root.accountController.cancel();
      if (root.focusDefault) root.focusDefault();
    }
  }

  NewChatDialog {
    objectName: "newChatDialog"
    id: dialog
    anchors.fill: parent
    visible: root.dialogController.open

    accounts: root.service ? root.service.accounts : []
    accountId: root.dialogController.accountId
    query: root.dialogController.query
    contacts: root.dialogController.contacts
    currentIndex: root.dialogController.currentIndex
    routeKey: root.routeKey

    onAccountChanged: id => root.dialogController.setAccount(id)
    onQueryEdited: text => root.dialogController.setQuery(text)
    onAccepted: (accountId, contactId) => root.dialogController.openConversation(accountId, contactId)
    onCancelled: root.dialogController.close()
  }
}
