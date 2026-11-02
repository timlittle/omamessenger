pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Rail.js" as Rail
import "../lib/Limits.js" as Limits

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
  // windowActive is whether the user can see the window at all: shown
  // and focused. The alias field exists only while this is also true
  // (see aliasLoader below): a field merely visible while the window
  // sits unfocused in the background - alt-tabbed away - is still a
  // live sink for a foreign Wayland clipboard offer, with no size or
  // time limit (see docs/decisions.md). Defaults true so a caller or
  // test that never sets it keeps the field exactly as it was before
  // this existed.
  property bool windowActive: true

  // chosen reports the account picked to rename.
  signal chosen(string accountId)
  // submitted reports the alias typed in, trimmed; "" clears it.
  signal submitted(string alias)
  // cancelled closes the dialog without changing anything.
  signal cancelled()

  // _alias exposes aliasLoader's own item, or null while the alias
  // step is not showing. A plain var, not an alias, since Loader.item
  // has no declared type for qmllint to check members against (the
  // same reasoning Composer.qml's own `input` property documents).
  property var _alias: aliasLoader.item
  // _aliasDraft is the field's own typed text, kept here rather than
  // only in the field itself: aliasLoader below destroys and recreates
  // it across a window-focus blip, so something outside it has to be
  // the value's real home, the same reasoning Composer.qml's own text
  // documents. Bound to currentAlias as a plain default rather than a
  // reset elsewhere: this component is itself recreated fresh for every
  // rename session (see Overlays.qml), and currentAlias never changes
  // again once that session starts, so the first keystroke severing
  // this binding (an ordinary QML binding, broken by assignment) never
  // loses a later prefill there was none of.
  property string _aliasDraft: root.currentAlias
  // _aliasHadFocus is true from the moment the real field last gained
  // activeFocus until it loses it for a reason other than the window
  // itself going inactive (a real blur: Tab, a click elsewhere) - see
  // the field's own onActiveFocusChanged below. windowActive turning
  // true again only restores focus to the field while this is also
  // true, rather than stealing it from whatever the user had instead
  // moved it to (Cancel or Save) before switching away.
  property bool _aliasHadFocus: false

  objectName: "renameAccount"
  anchors.fill: parent
  visible: root.open
  // Deferred with Qt.callLater, so by the time this runs, aliasLoader's
  // own active binding (bound to the same open-and-stage check) has
  // already created the field this focuses.
  onStageChanged: if (root.stage === "alias") Qt.callLater(() => { if (root._alias) root._alias.forceActiveFocus() })
  onOpenChanged: if (root.open && root.stage === "alias") Qt.callLater(() => { if (root._alias) root._alias.forceActiveFocus() })
  // Regaining the window's focus brings the field back only if it was
  // the thing actually focused when that focus was lost; see
  // _aliasHadFocus above.
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

      // aliasLoader holds the alias field, active only at the alias
      // step and only while the window is active (see windowActive
      // above): a field merely hidden with `visible:`, or merely
      // visible while the window sits unfocused in the background, is
      // still a live Qt object that keeps reading every foreign Wayland
      // clipboard offer, with no size or time limit, through the choose
      // step too or through an alt-tab (see docs/decisions.md), so it
      // must actually be destroyed while this dialog is picking which
      // account to rename, not just hidden behind ChoiceDialog.
      // _aliasDraft is what the field is seeded from and mirrors every
      // keystroke into, so recreating it on return from an alt-tab
      // loses nothing the user typed.
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
          // Qt's own right-click "Paste" menu would read the clipboard
          // in-process, with no size or time limit of its own;
          // routeKey below already routes Ctrl+V and Shift+Insert
          // through the helper's bounded clipboard.readText instead
          // (see docs/decisions.md), so this menu is removed outright.
          ContextMenu.menu: null
          Keys.priority: Keys.BeforeItem
          // Catches Qt's own paste shortcut (Ctrl+Shift+Insert, the
          // dedicated Paste key, …) ahead of the binding lookup below,
          // so a rebound field.paste in keys.conf can never bring back
          // this field's own unbounded native paste() (see
          // docs/decisions.md).
          Keys.onPressed: event => {
            if (event.matches(StandardKey.Paste) || event.key === Qt.Key_Paste) {
              if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
              event.accepted = true;
              return;
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }
          onAccepted: root.submitted(root._aliasDraft.trim())
          onTextChanged: root._aliasDraft = text
          // Losing focus because the window itself went inactive is not
          // a real blur: root.windowActive is already false by the time
          // this runs, since that is what emptied aliasLoader.active in
          // the first place, so _aliasHadFocus is left alone for that
          // case and only cleared for an actual Tab or a click
          // elsewhere.
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
