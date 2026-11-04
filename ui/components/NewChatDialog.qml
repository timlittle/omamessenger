pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Rail.js" as Rail
import "../lib/Limits.js" as Limits
import "../lib/Paste.js" as Paste

// New-chat modal: pick an account, search contacts, open one. An in-window
// scrim and card, not a native dialog, so it stays inside the FloatingWindow.
Item {
  id: root

  // accounts lists the signed-in Accounts to choose from.
  property var accounts: []
  // accountId is the account whose contacts are shown.
  property string accountId: ""
  // query is the search text, echoed back by queryEdited for the caller
  // to filter contacts with.
  property string query: ""
  // contacts are the Contacts to list, already filtered by the caller.
  property var contacts: []
  // currentIndex is the highlighted row in contacts.
  property int currentIndex: 0
  // knownServices: the helper's own services, from hello, for each
  // account button's label; falls back to the built-in labels when empty.
  property var knownServices: []
  // windowActive is whether the window is shown and focused; the search
  // field exists only while this is true, so it cannot sit as an
  // unfocused Wayland clipboard sink in the background (see
  // docs/decisions.md).
  property bool windowActive: true

  // searchField exposes the loaded search input, or null while the
  // window is inactive and the field does not exist.
  property var searchField: searchFieldLoader.item
  // routeKey intercepts a key press in the search field before the field
  // handles it; see Composer.qml for why this is a function property
  // rather than a signal carrying the KeyEvent.
  property var routeKey: null
  // _searchHadFocus tracks whether the field was genuinely focused
  // (not just inactive via windowActive) so focus can be restored
  // correctly when the window regains activation.
  property bool _searchHadFocus: false

  // accountChanged reports a new account chosen from the row of buttons.
  signal accountChanged(string id)
  // queryEdited reports the search text as the user types it.
  signal queryEdited(string text)
  // accepted reports the contact chosen to start a conversation with.
  signal accepted(string accountId, string contactId)
  // cancelled reports that the dialog should close without choosing.
  signal cancelled()

  // moveCurrent shifts the highlighted contact by delta, wrapping at the ends.
  function moveCurrent(delta: int): void {
    const count = root.contacts.length
    if (count === 0) {
      root.currentIndex = -1
      return
    }
    root.currentIndex = ((root.currentIndex + delta) % count + count) % count
  }

  // nextAccount selects the account after the current one, wrapping at the end.
  function nextAccount(): void {
    if (root.accounts.length === 0) return
    const ids = root.accounts.map(a => a.id)
    const at = ids.indexOf(root.accountId)
    root.accountChanged(ids[(at + 1) % ids.length])
  }

  // accept opens a conversation with the highlighted contact, if any.
  function accept(): void {
    const contact = root.contacts[root.currentIndex]
    if (!contact) return
    root.accepted(root.accountId, contact.remoteId)
  }

  // focusSearch moves keyboard focus into the search field, if it exists.
  function focusSearch(): void {
    if (root.searchField) root.searchField.forceActiveFocus()
  }

  anchors.fill: parent
  // Deferred so the loader has recreated the field before focusSearch runs.
  onWindowActiveChanged: if (root.windowActive && root._searchHadFocus) Qt.callLater(root.focusSearch)

  ModalCard {
    id: modal

    cardName: "newChatCard"
    // Wide enough for every account button, which names its service.
    cardWidth: Math.max(Style.space(360), accountRow.implicitWidth + modal.horizontalInsets)
    cardHeight: Style.space(420)
    onOutsideClicked: root.cancelled()

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      RowLayout {
        id: accountRow

        Layout.fillWidth: true
        spacing: Theme.spacing.controlGap

        Repeater {
          model: root.accounts

          Ui.Button {
            required property var modelData

            text: Rail.accountLabel(modelData, root.knownServices)
            selected: modelData.id === root.accountId
            focusable: true
            onClicked: root.accountChanged(modelData.id)
          }
        }
      }

      // The field is destroyed, not hidden, while the window is inactive
      // (see windowActive above); root.query survives the recreation.
      Loader {
        id: searchFieldLoader

        Layout.fillWidth: true
        active: root.windowActive
        onLoaded: root.searchField.forceActiveFocus()

        sourceComponent: Ui.TextField {
          objectName: "newChatSearchField"
          placeholderText: "Search contacts"
          text: root.query
          maximumLength: Limits.CONTACT_QUERY
          // Removed so paste always goes through the bounded routeKey path below.
          ContextMenu.menu: null

          Keys.priority: Keys.BeforeItem
          Keys.onPressed: event => {
            // Catch a native paste key ahead of the binding lookup below.
            if (Paste.isPasteKey(event.key, event.modifiers)) {
              if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
              event.accepted = true;
              return;
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }

          onTextChanged: root.queryEdited(text)
          // Only clear _searchHadFocus on a real blur, not a window deactivation.
          onActiveFocusChanged: {
            if (activeFocus) { root._searchHadFocus = true; return; }
            if (root.windowActive) root._searchHadFocus = false;
          }

          MiddleClickGuard {}
        }
      }

      ListView {
        Layout.fillWidth: true
        Layout.fillHeight: true
        clip: true
        model: root.contacts

        delegate: NewChatContactRow {
          required property var modelData
          required property int index

          width: ListView.view.width
          contact: modelData
          current: index === root.currentIndex
          onChosen: {
            root.currentIndex = index
            root.accept()
          }
        }
      }
    }
  }
}
