pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Rail.js" as Rail

// Asks which account to remove. Removing signs it out and deletes its
// chats from this computer, so the safe choice, Cancel, is highlighted
// first; j/k, Up/Down or a number highlight an account, Enter or y
// removes whichever is highlighted, n or Escape cancels.
Item {
  id: root

  // open shows the question.
  property bool open: false
  // accounts are the accounts that can be removed.
  property var accounts: []
  // removeIndex is the highlighted account: -1 for Cancel, the safe
  // default.
  property int removeIndex: -1
  // error is the last failure, shown under the choices.
  property string error: ""
  // knownServices: the helper's own services, from hello, for each
  // account's label; falls back to the built-in labels when empty.
  property var knownServices: []
  // routeKey intercepts a key press before this question's own control
  // handles it; see Composer.qml for why this is a function property,
  // not a signal.
  property var routeKey: null

  // chosen reports the account to remove.
  signal chosen(string accountId)
  // cancelled closes the question without removing anything.
  signal cancelled()

  objectName: "removeAccount"
  anchors.fill: parent
  visible: root.open
  focus: root.open

  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
  }

  ModalCard {
    onOutsideClicked: root.cancelled()

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        text: "Remove an account?"
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        text: root.accounts.length > 0
          ? "This signs the account out and deletes its chats from this computer. They stay on your phone. j/k or a number highlight one, Enter or y removes it."
          : "There are no accounts to remove."
        wrapMode: Text.WordWrap
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      Repeater {
        id: accountRepeater
        model: root.accounts

        Ui.Button {
          required property var modelData
          required property int index

          objectName: "removeButton-" + modelData.id
          Layout.fillWidth: true
          leftAlign: true
          text: (index < 9 ? (index + 1) + "  " : "") + "Remove " + Rail.accountDescription(modelData, root.knownServices)
          selected: index === root.removeIndex
          onClicked: root.chosen(modelData.id)
        }
      }

      Text {
        Layout.fillWidth: true
        visible: root.error !== ""
        text: root.error
        wrapMode: Text.WordWrap
        color: Color.urgent
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
      }

      Ui.Button {
        id: cancelButton

        objectName: "cancelButton"
        Layout.alignment: Qt.AlignRight
        text: "n  Cancel"
        selected: root.removeIndex === -1
        onClicked: root.cancelled()
      }
    }
  }
}
