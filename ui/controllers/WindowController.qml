import QtQuick
import "../lib/Navigation.js" as Navigation
import "../lib/Actions.js" as Actions
import "../lib/Keymap.js" as Keymap
import "../lib/Palette.js" as Palette
import "../lib/Rail.js" as Rail

// Owns the command palette, closing and quitting (which ask first), and
// the Escape chain. The palette runs commands through whichever
// controller owns them, and Escape needs to know what the others are
// showing, so it holds references to them, set once by whoever wires the
// controllers together.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // listController supplies conversations for the palette and is read for
  // the Escape chain.
  property var listController: null

  // conversationController is read and closed by the Escape chain, and
  // opens conversations chosen in the palette.
  property var conversationController: null

  // composerController is read and cleared by the Escape chain, and runs
  // the command palette's composer commands.
  property var composerController: null

  // photoViewerController is read and closed by the Escape chain.
  property var photoViewerController: null

  // reactionsController is read and closed by the Escape chain, and runs
  // the command palette's "react to the newest message" command.
  property var reactionsController: null

  // dialogController is read and closed by the Escape chain.
  property var dialogController: null

  // accountController adds accounts from the palette and is closed by the
  // Escape chain.
  property var accountController: null

  // paletteOpen shows the command palette.
  property bool paletteOpen: false

  // paletteMode is "commands" or "conversations": what the palette lists.
  property string paletteMode: "commands"

  // paletteQuery is the palette's search text.
  property string paletteQuery: ""

  // paletteIndex is the highlighted palette row.
  property int paletteIndex: 0

  // paletteResults are the matching commands or conversations, best first.
  readonly property var paletteResults: root.paletteMode === "conversations"
    ? Palette.search(root.listController ? root.listController.all : [], root.paletteQuery, (c) => c.title)
    : Palette.search(Keymap.commands(), root.paletteQuery, (c) => c.label)

  // paletteItems are paletteResults as rows to show: {label, detail, keys}.
  readonly property var paletteItems: root.paletteMode === "conversations"
    ? root.paletteResults.map((c) => ({ label: c.title, detail: Rail.serviceLabel(c.service, root.service ? root.service.services : []), keys: "" }))
    : root.paletteResults

  // confirmingClose shows the question asking what closing should do.
  property bool confirmingClose: false

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
      "palette.commands": () => root.openPalette("commands"),
      "palette.conversations": () => root.openPalette("conversations"),
      "palette.down": () => root.movePalette(1),
      "palette.up": () => root.movePalette(-1),
      "palette.accept": () => root.acceptPalette(root.paletteIndex),
      "window.hide": () => root.askToClose(),
      "app.quit": () => root.quit(),
      "escape": () => root._escape()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // openPalette shows the palette listing commands or conversations.
  function openPalette(mode: string): void {
    root.paletteMode = mode;
    root.paletteQuery = "";
    root.paletteIndex = 0;
    root.paletteOpen = true;
  }

  // closePalette hides the palette.
  function closePalette(): void {
    root.paletteOpen = false;
  }

  // setPaletteQuery filters the palette and highlights the best match.
  function setPaletteQuery(text: string): void {
    root.paletteQuery = text;
    root.paletteIndex = 0;
  }

  // movePalette moves the highlight, wrapping at the ends.
  function movePalette(delta: int): void {
    const count = root.paletteResults.length;
    if (count === 0) return;

    root.paletteIndex = ((root.paletteIndex + delta) % count + count) % count;
  }

  // acceptPalette runs the command, or opens the conversation, at index.
  function acceptPalette(index: int): void {
    const chosen = root.paletteResults[index];
    if (!chosen) return;

    root.closePalette();
    if (root.paletteMode === "conversations") root._openConversation(chosen);
    else root.runCommand(chosen.action);
  }

  // runCommand runs action through the controller that owns it.
  function runCommand(action: string): void {
    const controllers = [root.listController, root.conversationController, root.composerController,
      root.photoViewerController, root.reactionsController, root.dialogController, root.accountController, root];
    const owner = controllers.find((c) => c && c.handles(action));
    if (owner) owner.run(action);
  }

  // askToClose shows the close question instead of closing at once.
  function askToClose(): void {
    root.confirmingClose = true;
  }

  // keepInBackground answers the close question: hide the window and keep
  // notifying.
  function keepInBackground(): void {
    root.confirmingClose = false;
    root.hideRequested();
  }

  // quit stops the helper, and with it every notification, until the
  // window is opened again.
  function quit(): void {
    root.confirmingClose = false;
    if (root.service) root.service.quit();
    root.hideRequested();
  }

  // cancelClose leaves the window open.
  function cancelClose(): void {
    root.confirmingClose = false;
  }

  // _openConversation opens a conversation picked in the palette.
  function _openConversation(conversation: var): void {
    if (!root.conversationController) return;

    root.conversationController.open(conversation);
  }

  // _escape runs the one step Navigation.escapeAction says undoes the
  // innermost thing currently open.
  function _escape(): void {
    const steps = {
      "cancel-close": () => root.cancelClose(),
      "close-setup": () => { if (root.accountController) root.accountController.cancel(); },
      "close-palette": () => root.closePalette(),
      "close-viewer": () => { if (root.photoViewerController) root.photoViewerController.close(); },
      "close-reaction-picker": () => { if (root.reactionsController) root.reactionsController.closePicker(); },
      "close-dialog": () => { if (root.dialogController) root.dialogController.close(); },
      "clear-search": () => { if (root.listController) root.listController.clearSearch(); },
      "leave-search": () => { if (root.listController) root.listController.leaveSearch(); },
      "clear-attachment": () => { if (root.composerController) root.composerController.removeAttachment(); },
      "cancel-reply": () => { if (root.composerController) root.composerController.cancelReply(); },
      "leave-compose": () => { if (root.composerController) root.composerController.leaveComposeRequested(); },
      "close-conversation": () => { if (root.conversationController) root.conversationController.close(); },
      "hide-window": () => root.askToClose()
    };

    const step = steps[Navigation.escapeAction(root._navState())];
    if (step) step();
  }

  // _navState assembles what Navigation.keyContext and escapeAction need
  // to know, from the three controllers that each own one piece of it.
  function _navState(): var {
    const state = root.service ? root.service.uiState : { pane: "list", activeId: "" };

    return {
      confirmOpen: root.confirmingClose,
      setupOpen: root.accountController ? root.accountController.open || root.accountController.removing : false,
      viewerOpen: root.photoViewerController ? root.photoViewerController.viewerOpen : false,
      paletteOpen: root.paletteOpen,
      reactionPickerOpen: root.reactionsController ? root.reactionsController.pickerOpen : false,
      dialogOpen: root.dialogController ? root.dialogController.open : false,
      searchFocused: root.listController ? root.listController.searchFocused : false,
      composeFocused: root.composerController ? root.composerController.composeFocused : false,
      hasAttachment: root.composerController ? root.composerController.attachmentPath !== "" : false,
      replying: root.composerController ? root.composerController.replying : false,
      pane: state.pane,
      activeId: state.activeId,
      query: root.listController ? root.listController.query : ""
    };
  }
}
