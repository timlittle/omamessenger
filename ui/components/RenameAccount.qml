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
  // windowActive is whether the window is shown and focused; the alias field
  // exists only while this is true, so it is not a live clipboard sink while
  // backgrounded (see docs/decisions.md).
  property bool windowActive: true

  // chosen reports the account picked to rename.
  signal chosen(string accountId)
  // submitted reports the alias typed in, trimmed; "" clears it.
  signal submitted(string alias)
  // cancelled closes the dialog without changing anything.
  signal cancelled()

  // _alias exposes aliasLoader's own item, or null while the alias step is not showing.
  property var _alias: aliasLoader.item
  // _aliasDraft is the field's typed text, kept here since aliasLoader
  // destroys and recreates the field across a window-focus blip.
  property string _aliasDraft: root.currentAlias
  // _aliasHadFocus tracks whether the field held focus before losing it to
  // something other than the window going inactive, so refocus on return
  // does not steal focus the user moved elsewhere.
  property bool _aliasHadFocus: false

  objectName: "renameAccount"
  anchors.fill: parent
  visible: root.open
  // Deferred so aliasLoader has already created the field by the time this runs.
  onStageChanged: if (root.stage === "alias") Qt.callLater(() => { if (root._alias) root._alias.forceActiveFocus() })
  onOpenChanged: if (root.open && root.stage === "alias") Qt.callLater(() => { if (root._alias) root._alias.forceActiveFocus() })
  onWindowActiveChanged: if (root.windowActive && root._aliasHadFocus) Qt.callLater(() => { if (root._alias) root._alias.forceActiveFocus() })

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

      // aliasLoader holds the alias field, active only at the alias step and
      // while the window is active, so it is destroyed rather than merely
      // hidden otherwise (see docs/decisions.md). _aliasDraft preserves
      // what was typed across recreation.
      Loader {
        id: aliasLoader
        Layout.fillWidth: true
        active: root.open && root.stage === "alias" && root.windowActive

        sourceComponent: Ui.TextField {
          id: aliasFieldInner
          objectName: "aliasField"
          text: root._aliasDraft
          placeholderText: "e.g. Personal WhatsApp"
          maximumLength: Limits.ACCOUNT_ALIAS
          // The native paste menu reads the clipboard unbounded, so remove it;
          // routeKey below routes paste through the helper instead (see docs/decisions.md).
          ContextMenu.menu: null
          Keys.priority: Keys.BeforeItem
          // Catch any paste key ahead of routeKey, so a rebound field.paste
          // in keys.conf cannot resurrect the unbounded native paste().
          Keys.onPressed: event => {
            if (Paste.isPasteKey(event.key, event.modifiers)) {
              if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
              event.accepted = true;
              return;
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }
          onAccepted: root.submitted(root._aliasDraft.trim())
          onTextChanged: root._aliasDraft = text
          // Losing focus because the window went inactive is not a real blur,
          // so leave _aliasHadFocus alone for that case.
          onActiveFocusChanged: {
            if (activeFocus) { root._aliasHadFocus = true; return; }
            if (root.windowActive) root._aliasHadFocus = false;
          }

          MiddleClickGuard {}
        }
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
          onClicked: root.submitted(root._aliasDraft.trim())
        }
      }
    }
  }
}
