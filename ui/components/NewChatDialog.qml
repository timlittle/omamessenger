pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Rail.js" as Rail
import "../lib/Limits.js" as Limits

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
  // windowActive is whether the user can see the window at all: shown
  // and focused. The search field exists only while this is true (see
  // searchFieldLoader below): a field merely visible while the window
  // sits unfocused in the background - alt-tabbed away - is still a
  // live sink for a foreign Wayland clipboard offer, with no size or
  // time limit (see docs/decisions.md). Defaults true so a caller or
  // test that never sets it keeps the field exactly as it was before
  // this existed.
  property bool windowActive: true

  // searchField exposes the loaded search input, or null while the
  // window is inactive and it does not exist: a key router or a
  // focus-tracking caller must guard against null. A plain var, not an
  // alias, since the Loader's own item has no declared type for a
  // static checker to verify members against (the same reasoning
  // VoiceNoteController's own `_engine` documents).
  property var searchField: searchFieldLoader.item
  // routeKey intercepts a key press in the search field before the field
  // handles it; see Composer.qml for why this is a function property
  // rather than a signal carrying the KeyEvent.
  property var routeKey: null
  // _searchHadFocus is true from the moment the real field last gained
  // activeFocus until it loses it for a reason other than the window
  // itself going inactive (a real blur: Escape, Tab, a click
  // elsewhere) - see the field's own onActiveFocusChanged below.
  // windowActive turning true again only puts focus back in the field
  // while this is also true, rather than stealing it from whatever the
  // user had instead moved it to before switching away.
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
  // Regaining the window's focus brings the field back only if it was
  // the thing actually focused when that focus was lost; see
  // _searchHadFocus above. Deferred with Qt.callLater, so by the time
  // this runs, searchFieldLoader's own active binding has already
  // recreated the item this focuses (see AccountSetup.qml's own
  // _focusStep for the same reasoning).
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

      // The search field is destroyed, not merely hidden, exactly while
      // the window is inactive: a merely-visible, unfocused TextField
      // still reads every foreign Wayland clipboard offer, with no size
      // or time limit, for however long an alt-tab lasts (see
      // windowActive above and docs/decisions.md). root.query, owned by
      // DialogController, already survives this on its own (see
      // Overlays.qml), so recreating the field on return loses nothing
      // the user typed.
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
          // Qt's own right-click "Paste" menu would read the clipboard
          // in-process, with no size or time limit of its own; routeKey
          // below already routes Ctrl+V and Shift+Insert through the
          // helper's bounded clipboard.readText instead (see
          // docs/decisions.md), so this menu is removed outright.
          ContextMenu.menu: null

          Keys.priority: Keys.BeforeItem
          Keys.onPressed: event => {
            // Catches Qt's own paste shortcut (Ctrl+Shift+Insert, the
            // dedicated Paste key, …) ahead of the binding lookup below,
            // so a rebound field.paste in keys.conf can never bring back
            // this field's own unbounded native paste() (see
            // docs/decisions.md).
            if (event.matches(StandardKey.Paste) || event.key === Qt.Key_Paste) {
              if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
              event.accepted = true;
              return;
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }

          onTextChanged: root.queryEdited(text)
          // Losing focus because the window itself went inactive is not
          // a real blur: root.windowActive is already false by the time
          // this runs, since that is what emptied searchFieldLoader.active
          // in the first place, so _searchHadFocus is left alone for
          // that case and only cleared for an actual Escape, Tab or
          // outside click.
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
