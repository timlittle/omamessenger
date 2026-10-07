// Checks that Tab reaches the empty state's own "Add an account" button
// with real key events. Tab in the list context is bound to
// pane.conversation, which brings the open conversation column forward;
// with no account added yet there is nothing to bring forward, so its
// own handler must report that it did nothing rather than swallow the
// key, letting it fall through to the normal Qt focus chain the same way
// every other focusable control here is reached.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/controllers"
import "ui/lib/Keymap.js" as Keymap
import "ui/lib/Navigation.js" as Navigation
import "Check.js" as Check

ShellRoot {
  id: root

  // addAccountRequested records whether the empty state's own button
  // signal fired, once Tab and Enter have reached it.
  property bool addAccountRequested: false

  QtObject {
    id: service

    property var accounts: []
    property var services: []
    property var uiState: ({})
    property var pendingAuth: null
    property string status: "ready"
    property string detail: ""
    property int unreadTotal: 0

    // event is never emitted here: this fake answers every request
    // synchronously, but ListController listens for it, so it has to
    // exist.
    signal event(string name, var data)

    function start(): void {}
    function installHelper(): void {}
    function quit(): void {}

    // request answers every call with an empty list, since this
    // fixture's whole point is having no accounts and no conversations.
    // conversations.list answers with the array directly, the same
    // shape the real helper uses (see docs/api.md).
    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") { callback(null, []); return; }
      callback(null, {});
    }
  }

  ListController {
    id: listController
    service: service
  }

  ConversationController {
    id: conversationController
    service: service
    listController: listController
  }

  // routeKey is the same dispatch Panel.routeKey does: match the active
  // context, then hand the action to whichever controller owns it,
  // leaving the key unaccepted when that controller reports it did
  // nothing.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const context = Navigation.keyContext({
      confirmOpen: false, setupOpen: false, viewerOpen: false, paletteOpen: false,
      reactionPickerOpen: false, dialogOpen: false, searchFocused: listController.searchFocused,
      composeFocused: false, pane: conversationController.pane
    });
    const action = Keymap.match(context, key, modifiers, text);
    if (!action) return false;

    const controllers = [listController, conversationController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    return owner.run(action) !== false;
  }

  FloatingWindow {
    id: win
    implicitWidth: 400
    implicitHeight: 400
    visible: true

    Item {
      id: keyArea
      anchors.fill: parent
      focus: true

      Keys.onPressed: event => {
        if (root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
      }

      ListColumn {
        id: listColumn
        anchors.fill: parent

        query: listController.query
        model: listController.model
        selectedId: listController.selectedId
        showAll: listController.showAll
        hiddenCount: listController.hiddenCount
        showEmptyState: service.status === "ready" && service.accounts.length === 0
        routeKey: root.routeKey

        onQueryEdited: text => listController.setQuery(text)
        onAddAccountRequested: root.addAccountRequested = true
      }
    }
  }

  TestCase {
    id: t
    when: false
  }

  // A deliberately unreachable deadline: it only fires if the checks
  // below never run.
  Timer {
    running: true
    interval: 10000
    onTriggered: Check.fail("timed out before the checks finished")
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.run()
  }

  // run checks the button is there, that Tab from the key area reaches
  // it within a few presses, and that Enter on it reports the signal a
  // mouse click already would.
  function run(): void {
    const button = Check.find(listColumn, "addAccountButton");
    if (!button || !button.visible) return Check.fail("the empty state's Add an account button is not visible");

    keyArea.forceActiveFocus();
    for (let i = 0; i < 6 && !button.activeFocus; i++) t.keyClick(Qt.Key_Tab);
    if (!button.activeFocus)
      return Check.fail("Tab never reached \"Add an account\": pane.conversation may still be swallowing it with nothing to show");

    t.keyClick(Qt.Key_Return);
    if (!root.addAccountRequested) return Check.fail("Enter on the focused button did not report addAccountRequested");

    console.log("PASS EmptyState");
    Qt.exit(0);
  }
}
