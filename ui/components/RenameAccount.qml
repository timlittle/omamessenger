pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Rail.js" as Rail

// Sets an account's display name, the alias shown wherever that account
// is identified once more than one exists: the rail, the conversation
// list and the conversation header. Choosing which account comes first
// when there is more than one, the same shape as RemoveAccount: j/k,
// Up/Down or a number highlight one, Enter chooses it, n or Escape
// cancels. The alias field then starts with the account's current
// alias, or empty when it has none; Enter submits, saving an empty
// field as "" to clear the alias and fall back to the account's plain
// name everywhere it is shown, Escape cancels without changing
// anything.
Item {
  id: root

  // open shows the dialog, at whichever stage it is in.
  property bool open: false
  // stage is "choose" (picking which account) or "alias" (typing its
  // new name).
  property string stage: "choose"
  // accounts lists the accounts that can be renamed, for the choose
  // stage.
  property var accounts: []
  // chooseIndex is the choose stage's highlighted account: -1 for
  // Cancel, the same default RemoveAccount uses.
  property int chooseIndex: -1
  // currentAlias prefills the alias field once an account is chosen.
  property string currentAlias: ""
  // error is the last failure, shown under the current stage.
  property string error: ""
  // busy disables the alias field's Continue button while a request is
  // in flight.
  property bool busy: false
  // knownServices: the helper's own services, from hello, for each
  // account's label in the choose stage; falls back to the built-in
  // labels when empty.
  property var knownServices: []
  // routeKey intercepts a key press before a field or button handles
  // it; see Composer.qml for why this is a function property, not a
  // signal.
  property var routeKey: null

  // chosen reports the account picked to rename.
  signal chosen(string accountId)
  // submitted reports the alias typed in, trimmed; "" clears it.
  signal submitted(string alias)
  // cancelled closes the dialog without changing anything.
  signal cancelled()

  objectName: "renameAccount"
  anchors.fill: parent
  visible: root.open
  onStageChanged: if (root.stage === "alias") Qt.callLater(() => aliasField.forceActiveFocus())
  onOpenChanged: if (root.open && root.stage === "alias") Qt.callLater(() => aliasField.forceActiveFocus())

  ChoiceDialog {
    id: chooser

    open: root.open && root.stage === "choose"
    title: "Rename an account"
    description: root.accounts.length > 0
      ? "Choose which account to rename: j/k or a number highlight one, Enter chooses it."
      : "There are no accounts to rename."
    layout: "list"
    choices: root.accounts.map((account, index) => ({
      objectName: "renameButton-" + account.id,
      text: (index < 9 ? (index + 1) + "  " : "") + "Rename " + Rail.accountDescription(account, root.knownServices),
      selected: index === root.chooseIndex,
      value: account.id
    }))
    trailingChoices: [
      { objectName: "cancelButton", text: "n  Cancel", selected: root.chooseIndex === -1, value: "cancel" }
    ]
    routeKey: root.routeKey

    onPicked: value => {
      if (value === "cancel") root.cancelled();
      else root.chosen(value);
    }
  }

  ModalCard {
    visible: root.open && root.stage === "alias"
    cardWidth: Style.space(380)

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        text: "Rename account"
        textFormat: Text.PlainText
        color: Theme.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        text: "Shown wherever this account is identified, instead of its plain name. Leave it empty to clear it."
        wrapMode: Text.WordWrap
        textFormat: Text.PlainText
        color: Util.alpha(Theme.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }

      Ui.TextField {
        id: aliasField
        objectName: "aliasField"
        Layout.fillWidth: true
        text: root.currentAlias
        placeholderText: "e.g. Personal WhatsApp"
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: event => { if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true }
        onAccepted: root.submitted(text.trim())
      }

      Text {
        Layout.fillWidth: true
        visible: root.error !== ""
        text: root.error
        textFormat: Text.PlainText
        wrapMode: Text.WordWrap
        color: Theme.urgent
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
      }

      RowLayout {
        Layout.alignment: Qt.AlignRight
        spacing: Theme.spacing.controlGap

        Ui.Button {
          objectName: "cancelButton"
          text: "Cancel"
          onClicked: root.cancelled()
        }

        Ui.Button {
          objectName: "saveButton"
          enabled: !root.busy
          text: "Save"
          focusable: true
          selected: true
          onClicked: root.submitted(aliasField.text.trim())
        }
      }
    }
  }
}
