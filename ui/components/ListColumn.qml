pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Keymap.js" as Keymap
import "../lib/Limits.js" as Limits
import "../lib/Paste.js" as Paste

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
  // windowActive is whether the window is shown and focused. The search
  // field exists only while this is true, to avoid an unfocused window
  // keeping a live clipboard sink open (see docs/decisions.md).
  // Defaults true so a caller that never sets it keeps prior behaviour.
  property bool windowActive: true

  // _editingSearch is true from entering the search field until a real
  // blur: the real TextField exists only then, since a visible but
  // untouched field is itself a clipboard sink (see docs/decisions.md).
  // Survives a windowActive blip so searching resumes after refocus.
  property bool _editingSearch: false

  // focusSearch starts editing the search field, the way a click on its
  // placeholder does. Focuses it at once if loaded, otherwise once the
  // Loader's onLoaded catches up.
  function focusSearch(): void {
    root._editingSearch = true;
    if (root.searchField) root.searchField.forceActiveFocus();
  }

  // searchField exposes the loaded field itself, or null while the list
  // column is not shown and the field does not exist: a key router or a
  // focus-tracking Connections must guard against null (see
  // MessengerLayout.qml). A plain var, not an alias, since the Loader's
  // own item has no declared type for qmllint to check members against
  // (the same reasoning VoiceNoteController's own `_engine` documents).
  property var searchField: searchFieldLoader.item

  // queryEdited reports the search text as the user types it.
  signal queryEdited(string text)
  // activated reports a row the user picked to open.
  signal activated(string id)
  // pinReordered reports a pinned row dropped after a drag, naming
  // where it landed (see ConversationList.pinDropped).
  signal pinReordered(string id, int toIndex)
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

  // searchPlaceholder shows the current query, or "Search", as plain
  // text while not being edited; clicking it starts editing.
  Rectangle {
    id: searchPlaceholder
    objectName: "searchPlaceholder"
    Layout.fillWidth: true
    Layout.preferredHeight: placeholderText.implicitHeight + Theme.spacing.inputPaddingY * 2
    visible: !searchFieldLoader.active
    radius: Style.cornerRadius
    color: Style.normalFill

    Text {
      id: placeholderText
      anchors.verticalCenter: parent.verticalCenter
      anchors.left: parent.left
      anchors.right: parent.right
      anchors.leftMargin: Theme.spacing.controlPaddingX
      text: root.query.length > 0 ? root.query : "Search"
      textFormat: Text.PlainText
      elide: Text.ElideRight
      color: root.query.length > 0 ? Theme.foreground : Util.alpha(Theme.foreground, 0.4)
      font.family: Theme.font.family
      font.pixelSize: Theme.font.body
    }

    MouseArea {
      anchors.fill: parent
      onClicked: root.focusSearch()
    }
  }

  // The search field is destroyed, not merely hidden, while it is not
  // being edited or the window is not active: a still-alive TextInput
  // keeps reading Wayland clipboard changes with no limit (see
  // docs/decisions.md). Its text mirrors root.query one-way, so
  // recreating it loses nothing.
  Loader {
    id: searchFieldLoader
    Layout.fillWidth: true
    active: root.visible && root._editingSearch && root.windowActive
    onLoaded: if (root._editingSearch) root.searchField.forceActiveFocus()

    sourceComponent: Ui.TextField {
      objectName: "searchField"
      placeholderText: "Search"
      text: root.query
      maximumLength: Limits.SEARCH_QUERY
      // Qt's own right-click paste menu reads the clipboard in-process
      // with no limit; routed through the helper instead (see
      // docs/decisions.md), so remove it.
      ContextMenu.menu: null

      Keys.priority: Keys.BeforeItem
      Keys.onPressed: event => {
        // Catches a native paste key ahead of the binding lookup so it
        // can never trigger the field's own unbounded paste().
        if (Paste.isPasteKey(event.key, event.modifiers)) {
          if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
          event.accepted = true;
          return;
        }
        if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
      }

      onTextChanged: root.queryEdited(text)
      // A real blur (Escape, Tab, click elsewhere) ends editing; losing
      // focus because the window went inactive does not, since
      // windowActive is already false by then.
      onActiveFocusChanged: if (!activeFocus && root.windowActive) root._editingSearch = false

      MiddleClickGuard {}
    }
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
      textFormat: Text.PlainText
      wrapMode: Text.WordWrap
      color: Util.alpha(Theme.foreground, 0.7)
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
      textFormat: Text.PlainText
      color: Theme.accent
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
    onPinDropped: (id, toIndex) => root.pinReordered(id, toIndex)
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
      textFormat: Text.PlainText
      elide: Text.ElideRight
      color: Util.alpha(Theme.foreground, 0.7)
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
