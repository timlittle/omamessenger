import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The search field, conversation list and its "older chats" fold, plus
// the empty state shown before any account is added. Takes its data
// through properties and reports user intent through signals, like any
// other view; MessengerLayout wires it to the controllers.
ColumnLayout {
  id: root

  // query is the list's current search text.
  property string query: ""
  // model is the conversation rows the list shows.
  property var model: null
  // selectedId is the cursor's current row, by id.
  property string selectedId: ""
  // nowMs is the current time, for relative timestamps.
  property real nowMs: 0
  // accountNames names each row's account, for a multi-account service.
  property var accountNames: ({})
  // multiAccountServices lists which services have more than one account.
  property var multiAccountServices: []
  // showEmptyState is true once the helper is ready and has no accounts
  // yet, in place of the list.
  property bool showEmptyState: false
  // showOlder is true while chats older than a month are shown.
  property bool showOlder: false
  // hiddenCount is how many older chats a non-empty search still hides.
  property int hiddenCount: 0
  // routeKey is forwarded to the search field; see Composer.qml for why
  // a key router intercepts through a function property, not a signal.
  property var routeKey: null

  // searchField exposes the field itself so a key router can focus it.
  property alias searchField: searchField

  // queryEdited reports the search text as the user types it.
  signal queryEdited(string text)
  // activated reports a row the user picked to open.
  signal activated(string id)
  // addAccountRequested asks the caller to start adding an account, from
  // the empty state's button.
  signal addAccountRequested()
  // showOlderToggled asks the caller to flip whether older chats show.
  signal showOlderToggled()

  spacing: Theme.spacing.sm

  Ui.TextField {
    id: searchField
    Layout.fillWidth: true
    placeholderText: "Search"
    text: root.query

    Keys.priority: Keys.BeforeItem
    Keys.onPressed: event => {
      if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
    }

    onTextChanged: root.queryEdited(text)
  }

  // With no accounts yet, the list says how to add one.
  ColumnLayout {
    objectName: "noAccounts"
    Layout.fillWidth: true
    visible: root.showEmptyState
    spacing: Theme.spacing.sm

    Text {
      Layout.fillWidth: true
      text: "No accounts yet. Add an account to see your chats here."
      wrapMode: Text.WordWrap
      color: Util.alpha(Color.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.body }
    }

    Ui.Button {
      objectName: "addAccountButton"
      text: "Add an account"
      focusable: true
      onClicked: root.addAccountRequested()
    }
  }

  ConversationList {
    id: list
    Layout.fillWidth: true
    Layout.fillHeight: true

    model: root.model
    selectedId: root.selectedId
    query: root.query
    nowMs: root.nowMs
    accountNames: root.accountNames
    multiAccountServices: root.multiAccountServices

    onActivated: id => root.activated(id)
  }

  // Chats older than a month are hidden until asked for.
  RowLayout {
    objectName: "olderChats"
    Layout.fillWidth: true
    visible: root.hiddenCount > 0 || (root.showOlder && root.query === "")
    spacing: Theme.spacing.sm

    Text {
      Layout.fillWidth: true
      text: root.showOlder ? "Showing chats older than a month"
        : root.hiddenCount + (root.hiddenCount === 1 ? " older chat hidden" : " older chats hidden")
      elide: Text.ElideRight
      color: Util.alpha(Color.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
    }

    Ui.Button {
      objectName: "olderChatsButton"
      text: root.showOlder ? "Hide" : "Show"
      focusable: true
      onClicked: root.showOlderToggled()
    }
  }
}
