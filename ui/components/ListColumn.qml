import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Keymap.js" as Keymap

// The search field, conversation list and its show-all fold, plus the
// empty state shown before any account is added. Takes its data through
// properties and reports user intent through signals, like any other
// view; MessengerLayout wires it to the controllers.
ColumnLayout {
  id: root

  // query is the list's current search text.
  property string query: ""
  // model is the conversation rows the list shows.
  property var model: null
  // selectedId is the cursor's current row, by id.
  property string selectedId: ""
  // accountNames names each row's account, for a multi-account service.
  property var accountNames: ({})
  // multiAccountServices lists which services have more than one account.
  property var multiAccountServices: []
  // accountColors: accountId -> colour tag, for each row's account stripe.
  property var accountColors: ({})
  // showAccountColors: true once more than one account exists.
  property bool showAccountColors: false
  // showEmptyState is true once the helper is ready and has no accounts
  // yet, in place of the list.
  property bool showEmptyState: false
  // showAll is true while every chat the standard list folds away is shown.
  property bool showAll: false
  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), read for the "Leave" button's label.
  property var bindings: Keymap.BINDINGS
  // hiddenCount is how many chats the standard list hides right now.
  property int hiddenCount: 0
  // unreadView is true while the all-unreads view is showing, overriding
  // the rail filter.
  property bool unreadView: false
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
  // showAllToggled asks the caller to flip whether every folded-away chat
  // (older, hidden or archived) shows.
  signal showAllToggled()
  // unreadViewLeft asks the caller to turn off the all-unreads view, from
  // the header's own button; the shortcut and Esc reach the same effect.
  signal unreadViewLeft()

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

  // The "Unread" header: shown while the all-unreads view (Ctrl+Shift+A)
  // overrides the rail filter below. Bold, accent-coloured text pairs with
  // the "Leave" button's own label, so the state never relies on colour
  // alone.
  RowLayout {
    objectName: "unreadHeaderRow"
    Layout.fillWidth: true
    visible: root.unreadView
    spacing: Theme.spacing.sm

    Text {
      Layout.fillWidth: true
      text: "Unread"
      color: Color.accent
      font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
    }

    Ui.Button {
      objectName: "leaveUnreadViewButton"
      text: "Leave (" + Keymap.keyFor("list.unread", root.bindings) + ")"
      focusable: true
      onClicked: root.unreadViewLeft()
    }
  }

  ConversationList {
    id: list
    Layout.fillWidth: true
    Layout.fillHeight: true

    model: root.model
    selectedId: root.selectedId
    query: root.query
    accountNames: root.accountNames
    multiAccountServices: root.multiAccountServices
    accountColors: root.accountColors
    showAccountColors: root.showAccountColors
    unreadView: root.unreadView

    onActivated: id => root.activated(id)
  }

  // Chats the standard list folds away (older than a month, hidden by the
  // user, or archived with the service) stay out of sight until asked for.
  RowLayout {
    objectName: "showAllRow"
    Layout.fillWidth: true
    visible: !root.unreadView && (root.hiddenCount > 0 || (root.showAll && root.query === ""))
    spacing: Theme.spacing.sm

    Text {
      Layout.fillWidth: true
      text: root.showAll ? "Showing all chats"
        : root.hiddenCount + (root.hiddenCount === 1 ? " chat hidden" : " chats hidden")
      elide: Text.ElideRight
      color: Util.alpha(Color.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
    }

    Ui.Button {
      objectName: "showAllButton"
      text: root.showAll ? "Show fewer" : "Show all"
      focusable: true
      onClicked: root.showAllToggled()
    }
  }
}
