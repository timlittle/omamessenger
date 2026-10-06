pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

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

  // searchField exposes the search input so a key router can intercept
  // navigation keys before the field types them, the same way Composer
  // exposes its own input.
  property alias searchField: searchField
  // routeKey intercepts a key press in the search field before the field
  // handles it; see Composer.qml for why this is a function property
  // rather than a signal carrying the KeyEvent.
  property var routeKey: null

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

  // focusSearch moves keyboard focus into the search field.
  function focusSearch(): void {
    searchField.forceActiveFocus()
  }

  anchors.fill: parent

  Rectangle {
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
      onClicked: root.cancelled()
    }

    Ui.BorderSurface {
      id: card
      anchors.centerIn: parent
      width: Style.space(360)
      height: Style.space(420)
      color: Theme.popups.background
      borderSpec: Border.flat(Theme.popups.border, Style.normalBorderWidth)
      radius: Style.cornerRadius
      padding: Theme.spacing.panelPadding

      MouseArea {
        anchors.fill: parent
        onClicked: () => {}
      }

      ColumnLayout {
        anchors.fill: parent
        anchors.topMargin: card.contentTopInset
        anchors.rightMargin: card.contentRightInset
        anchors.bottomMargin: card.contentBottomInset
        anchors.leftMargin: card.contentLeftInset
        spacing: Theme.spacing.md

        RowLayout {
          Layout.fillWidth: true
          spacing: Theme.spacing.controlGap

          Repeater {
            model: root.accounts

            Ui.Button {
              required property var modelData

              text: modelData.name
              selected: modelData.id === root.accountId
              focusable: true
              onClicked: root.accountChanged(modelData.id)
            }
          }
        }

        Ui.TextField {
          id: searchField
          Layout.fillWidth: true
          placeholderText: "Search contacts"
          text: root.query

          Keys.priority: Keys.BeforeItem
          Keys.onPressed: event => {
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }

          onTextChanged: root.queryEdited(text)
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
}
