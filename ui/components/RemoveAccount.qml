pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Rail.js" as Rail

// Asks which account to remove. Removing signs it out and deletes its
// chats from this computer, so the safe choice, Cancel, has focus first;
// Tab moves to each account and Escape cancels.
Item {
  id: root

  // open shows the question.
  property bool open: false
  // accounts are the accounts that can be removed.
  property var accounts: []
  // error is the last failure, shown under the choices.
  property string error: ""
  // knownServices: the helper's own services, from hello, for each
  // account's label; falls back to the built-in labels when empty.
  property var knownServices: []

  // chosen reports the account to remove.
  signal chosen(string accountId)
  // cancelled closes the question without removing anything.
  signal cancelled()

  objectName: "removeAccount"
  anchors.fill: parent
  visible: root.open
  // Focus waits a turn of the event loop: an item still hidden when open
  // changes cannot take focus.
  onOpenChanged: if (root.open) Qt.callLater(() => cancelButton.forceActiveFocus())

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
          ? "This signs the account out and deletes its chats from this computer. They stay on your phone."
          : "There are no accounts to remove."
        wrapMode: Text.WordWrap
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      Repeater {
        model: root.accounts

        Ui.Button {
          required property var modelData

          Layout.fillWidth: true
          leftAlign: true
          text: "Remove " + Rail.accountDescription(modelData, root.knownServices)
          focusable: true
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

        Layout.alignment: Qt.AlignRight
        text: "Cancel"
        focusable: true
        selected: true
        onClicked: root.cancelled()
      }
    }
  }
}
