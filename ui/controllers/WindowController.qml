import QtQuick
import "../lib/Navigation.js" as Navigation
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc

// Owns the shortcut help sheet, hiding the window, demo data and the
// Escape chain. Escape needs to know what the other three controllers are
// showing, so it holds references to them, set once by whoever wires the
// controllers together.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // listController is read for the Escape chain and for demo.inject's
  // target conversation.
  property var listController: null

  // conversationController is read and closed by the Escape chain.
  property var conversationController: null

  // dialogController is read and closed by the Escape chain.
  property var dialogController: null

  // helpOpen shows the shortcut help sheet when true.
  property bool helpOpen: false

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // hideRequested asks the caller to hide the window. The panel turns
  // this into shell.hide, since only it holds the shell facade.
  signal hideRequested()

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "window";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "help.toggle": () => { root.helpOpen = !root.helpOpen; },
      "help.close": () => { root.helpOpen = false; },
      "window.hide": () => root.hideRequested(),
      "escape": () => root._escape(),
      "demo.inject": () => root._injectDemo()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // _escape runs the one step Navigation.escapeAction says undoes the
  // innermost thing currently open.
  function _escape(): void {
    const steps = {
      "close-help": () => { root.helpOpen = false; },
      "close-dialog": () => { if (root.dialogController) root.dialogController.close(); },
      "clear-search": () => { if (root.listController) root.listController.clearSearch(); },
      "leave-search": () => { if (root.listController) root.listController.leaveSearch(); },
      "leave-compose": () => { if (root.conversationController) root.conversationController.leaveComposeRequested(); },
      "close-conversation": () => { if (root.conversationController) root.conversationController.close(); },
      "hide-window": () => root.hideRequested()
    };

    const step = steps[Navigation.escapeAction(root._navState())];
    if (step) step();
  }

  // _navState assembles what Navigation.keyContext and escapeAction need
  // to know, from the three controllers that each own one piece of it.
  function _navState(): var {
    const state = root.service ? root.service.uiState : { pane: "list", activeId: "" };

    return {
      helpOpen: root.helpOpen,
      dialogOpen: root.dialogController ? root.dialogController.open : false,
      searchFocused: root.listController ? root.listController.searchFocused : false,
      composeFocused: root.conversationController ? root.conversationController.composeFocused : false,
      pane: state.pane,
      activeId: state.activeId,
      query: root.listController ? root.listController.query : ""
    };
  }

  // _injectDemo delivers a scripted demo message into the list's selected
  // conversation, while the helper is seeded with demo data.
  function _injectDemo(): void {
    if (!root.service || !root.service.demo || !root.listController || !root.listController.selectedId) return;

    root.service.request("demo.inject", { conversationId: root.listController.selectedId }, function(error) {
      if (error) root.lastError = Rpc.errorText(error);
    });
  }
}
