import QtQuick
import "../lib/Navigation.js" as Navigation
import "../lib/Actions.js" as Actions
import "../lib/Keymap.js" as Keymap
import "../lib/KeyBindings.js" as KeyBindings
import "../lib/Palette.js" as Palette
import "../lib/Rail.js" as Rail
import "../lib/Rpc.js" as Rpc

// Owns the command palette, closing and quitting (which ask first), and
// the Escape chain. The palette runs commands through whichever
// controller owns them, and Escape needs to know what the others are
// showing, so it holds references to them, set once by whoever wires the
// controllers together.
//
// Item rather than QtObject: it holds the debounced message search timer
// below.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // listController supplies conversations for the palette and is read for
  // the Escape chain.
  property var listController: null

  // conversationController is read and closed by the Escape chain, opens
  // conversations chosen in the palette, and opens and highlights a
  // message chosen from its "Messages" section.
  property var conversationController: null

  // composerController is read and cleared by the Escape chain, and runs
  // the command palette's composer commands.
  property var composerController: null

  // photoViewerController is read and closed by the Escape chain.
  property var photoViewerController: null

  // reactionsController is read and closed by the Escape chain, and runs
  // the command palette's "react to the highlighted message" command.
  property var reactionsController: null

  // deleteController is read and closed by the Escape chain, and runs
  // the command palette's "delete the highlighted message" command.
  property var deleteController: null

  // dialogController is read and closed by the Escape chain.
  property var dialogController: null

  // accountController adds accounts from the palette and is closed by the
  // Escape chain.
  property var accountController: null

  // paletteOpen shows the command palette.
  property bool paletteOpen: false

  // paletteMode is "commands", "conversations", "links" or
  // "keyBindings": what the palette lists.
  property string paletteMode: "commands"

  // bindings are the effective key bindings (defaults merged with the
  // user's keys.conf overrides), read for the palette's own commands
  // list and its "Show key bindings" dump.
  readonly property var bindings: (root.service && root.service.effectiveBindings) || Keymap.BINDINGS

  // paletteQuery is the palette's search text.
  property string paletteQuery: ""

  // paletteIndex is the highlighted row, counting the "Conversations"
  // section first and the "Messages" section after it, in "conversations"
  // mode; just paletteResults in every other mode.
  property int paletteIndex: 0

  // linkChoices are the URLs offered when the palette opens in "links"
  // mode: the highlighted message's own links, set by openLinkChooser.
  property var linkChoices: []

  // _messageSearchResults is the debounced message search's last answer:
  // conversations.list's own result shape, the same request Ctrl+G used
  // before it became an alias for this palette (see _searchMessages).
  property var _messageSearchResults: []

  // paletteResults are the matching commands, conversations or links,
  // best first. In "conversations" mode this is the palette's
  // "Conversations" section alone, filtered and ordered locally from
  // whatever the list has already loaded; paletteMessageResults is the
  // separate "Messages" section, from the server.
  readonly property var paletteResults: root.paletteMode === "conversations"
    ? Palette.search(root.listController ? Palette.conversationOrder(root.listController.all) : [], root.paletteQuery, (c) => c.title)
    : root.paletteMode === "links"
    ? Palette.search(root.linkChoices.map((url) => ({ url })), root.paletteQuery, (c) => c.url)
    : root.paletteMode === "keyBindings"
    ? Palette.search(KeyBindings.rows(root.bindings, root._keyBindingConflicts, root._keyBindingErrors), root.paletteQuery, (r) => r.label)
    : Palette.search(Keymap.commands(root.bindings), root.paletteQuery, (c) => c.label)

  // paletteMessageResults is the palette's "Messages" section: messages
  // whose text matched paletteQuery, newest or best match first, empty
  // outside "conversations" mode or before the debounce has fired.
  readonly property var paletteMessageResults: root.paletteMode === "conversations"
    ? Palette.messageRows(root._messageSearchResults) : []

  // _keyBindingConflicts/_keyBindingErrors read Service's report of
  // keys.conf, or nothing for a service too old to have one.
  readonly property var _keyBindingConflicts: (root.service && root.service.keyBindingConflicts) || []
  readonly property var _keyBindingErrors: (root.service && root.service.keyBindingErrors) || []

  // paletteItems are paletteResults, and in "conversations" mode
  // paletteMessageResults after them, as rows to show: {label, detail,
  // keys, unread, section}. Conversations carry their unread count so
  // PaletteRow can show the same badge the list does; a message row
  // shows its sender and snippet as the label and its conversation as
  // the detail.
  readonly property var paletteItems: root.paletteMode === "conversations"
    ? root.paletteResults.map((c) => ({
        label: c.title, detail: Rail.serviceLabel(c.service, root.service ? root.service.services : []),
        keys: "", unread: c.unread ?? 0, section: "conversation"
      })).concat(root.paletteMessageResults.map((m) => ({
        label: (m.sender ? m.sender + ": " : "") + m.snippet, detail: m.conversationTitle,
        keys: "", unread: 0, section: "message"
      })))
    : root.paletteMode === "links"
    ? root.paletteResults.map((c) => ({ label: "Open link: " + c.url, detail: "", keys: "" }))
    : root.paletteResults

  // confirmingClose shows the question asking what closing should do.
  property bool confirmingClose: false

  // confirmIndex is the close question's highlighted choice: 0 Cancel, 1
  // Quit, 2 Keep in background (its safe default), left to right.
  property int confirmIndex: 2

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // doctorOpen shows the health check report.
  property bool doctorOpen: false

  // doctorChecks are the report's checks: [{name, ok, detail}], from the
  // helper's helper.doctor method.
  property var doctorChecks: []

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
      // Ctrl+G used to focus the list's own inline search field; it now
      // opens this same palette, so there is one search to reach for
      // instead of two. See docs/shortcuts.md.
      "search.focus": () => root.openPalette("conversations"),
      "palette.down": () => root.movePalette(1),
      "palette.up": () => root.movePalette(-1),
      "palette.accept": () => root.acceptPalette(root.paletteIndex),
      "window.hide": () => root.askToClose(),
      "app.quit": () => root.quit(),
      "helper.retryInstall": () => root.retryInstall(),
      "helper.doctor": () => root.runDoctor(),
      "keys.openConfig": () => { if (root.service) root.service.openKeyConfigFile(); },
      "keys.showBindings": () => root.openPalette("keyBindings"),
      "settings.toggleReadReceipts": () => { if (root.service) root.service.toggleReadReceipts(); },
      "close.left": () => { root.confirmIndex = Math.max(0, root.confirmIndex - 1); },
      "close.right": () => { root.confirmIndex = Math.min(2, root.confirmIndex + 1); },
      "close.accept": () => root._acceptClose(),
      "close.cancel": () => root.cancelClose(),
      "close.quit": () => root.quit(),
      "close.keep": () => root.keepInBackground(),
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
    root._messageSearchResults = [];
    root.paletteOpen = true;
  }

  // openLinkChooser shows the palette listing urls, for o on a message
  // that carries more than one link: picking one opens it, the same as
  // the single-link case opens straight away without asking.
  function openLinkChooser(urls: var): void {
    root.linkChoices = urls;
    root.openPalette("links");
  }

  // retryInstall retries installing the helper. Guarded on the status
  // the Retry button itself only shows for, so the global key and the
  // palette command both do nothing at any other time.
  function retryInstall(): void {
    if (root.service && root.service.status === "installFailed") root.service.installHelper();
  }

  // closePalette hides the palette and stops any message search still
  // waiting on its debounce, so a reply for a closed palette never
  // applies to whatever opens next.
  function closePalette(): void {
    root.paletteOpen = false;
    messageSearchTimer.stop();
  }

  // runDoctor asks the helper for its own health report and shows it once
  // it answers; a request that fails surfaces the same way any other
  // one does, through lastError.
  function runDoctor(): void {
    if (!root.service) return;

    root.service.request("helper.doctor", {}, function(error, result) {
      if (error) { root.lastError = Rpc.errorText(error); return; }

      root.doctorChecks = result.checks ?? [];
      root.doctorOpen = true;
    });
  }

  // closeDoctor hides the health check report.
  function closeDoctor(): void {
    root.doctorOpen = false;
  }

  // setPaletteQuery filters the palette and highlights the best match.
  // In "conversations" mode the "Conversations" section narrows at once,
  // from what is already loaded, while the "Messages" section waits for
  // messageSearchTimer's debounce before asking the helper, so typing
  // fast never fires one request per keystroke.
  function setPaletteQuery(text: string): void {
    root.paletteQuery = text;
    root.paletteIndex = 0;
    if (root.paletteMode === "conversations") messageSearchTimer.restart();
  }

  // _searchMessages asks the helper for the "Messages" section, once
  // messageSearchTimer's debounce fires. A reply for a query the palette
  // has since moved on from, or closed, is dropped.
  function _searchMessages(): void {
    const query = root.paletteQuery;
    if (!root.paletteOpen || root.paletteMode !== "conversations" || !query) {
      root._messageSearchResults = [];
      return;
    }

    root.service.request("conversations.list", { query: query }, function(error, result) {
      if (Palette.staleMessageSearch(query, root.paletteQuery) || !root.paletteOpen) return;
      if (error) { root.lastError = Rpc.errorText(error); return; }

      root._messageSearchResults = result ?? [];
    });
  }

  // movePalette moves the highlight, wrapping at the ends, across both
  // the "Conversations" and "Messages" sections in "conversations" mode.
  function movePalette(delta: int): void {
    const count = root.paletteResults.length + root.paletteMessageResults.length;
    if (count === 0) return;

    root.paletteIndex = ((root.paletteIndex + delta) % count + count) % count;
  }

  // acceptPalette runs the command, opens the conversation or the
  // matched message, or opens the link, at index. "keyBindings" rows are
  // informational only: Enter on one just closes the palette, same as
  // clicking outside it would.
  function acceptPalette(index: int): void {
    if (root.paletteMode === "conversations" && index >= root.paletteResults.length) {
      const message = root.paletteMessageResults[index - root.paletteResults.length];
      if (!message) return;

      root.closePalette();
      root._openMessage(message);
      return;
    }

    const chosen = root.paletteResults[index];
    if (!chosen) return;

    root.closePalette();
    if (root.paletteMode === "conversations") root._openConversation(chosen);
    else if (root.paletteMode === "links") Qt.openUrlExternally(chosen.url);
    else if (root.paletteMode === "keyBindings") { /* informational only */ }
    else root.runCommand(chosen.action);
  }

  // _openMessage opens the conversation a "Messages" row belongs to and
  // highlights the matched message, so Enter there lands on what was
  // actually found rather than just the chat it is in.
  function _openMessage(message: var): void {
    if (!root.conversationController || !root.listController) return;

    const conversation = root.listController.findConversation(message.conversationId);
    if (conversation) root.conversationController.openMessage(conversation, message.messageId);
  }

  // runCommand runs action through the controller that owns it.
  function runCommand(action: string): void {
    const controllers = [root.listController, root.conversationController, root.composerController,
      root.photoViewerController, root.reactionsController, root.deleteController, root.dialogController, root.accountController, root];
    const owner = controllers.find((c) => c && c.handles(action));
    if (owner) owner.run(action);
  }

  // askToClose shows the close question instead of closing at once,
  // highlighted on its safe default, Keep in background.
  function askToClose(): void {
    root.confirmingClose = true;
    root.confirmIndex = 2;
  }

  // _acceptClose runs whichever choice is highlighted, for Enter.
  function _acceptClose(): void {
    if (root.confirmIndex === 0) root.cancelClose();
    else if (root.confirmIndex === 1) root.quit();
    else root.keepInBackground();
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

  // _leaveCompose blurs the composer and resets the highlighted message
  // to the newest one, so scroll mode always starts there once writing
  // stops, the same as opening a conversation does.
  function _leaveCompose(): void {
    if (root.composerController) root.composerController.leaveComposeRequested();
    if (root.conversationController) root.conversationController.resetHighlight();
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
      "close-doctor": () => root.closeDoctor(),
      "close-reaction-picker": () => { if (root.reactionsController) root.reactionsController.closePicker(); },
      "close-delete-confirm": () => { if (root.deleteController) root.deleteController.close(); },
      "close-dialog": () => { if (root.dialogController) root.dialogController.close(); },
      "clear-search": () => { if (root.listController) root.listController.clearSearch(); },
      "leave-search": () => { if (root.listController) root.listController.leaveSearch(); },
      "leave-unread-view": () => { if (root.listController) root.listController.setUnreadView(false); },
      "clear-attachment": () => { if (root.composerController) root.composerController.removeAttachment(); },
      "cancel-reply": () => { if (root.composerController) root.composerController.cancelReply(); },
      "leave-compose": () => root._leaveCompose(),
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
      setupContext: root.accountController ? root.accountController.navContext : "",
      viewerOpen: root.photoViewerController ? root.photoViewerController.viewerOpen : false,
      doctorOpen: root.doctorOpen,
      paletteOpen: root.paletteOpen,
      reactionPickerOpen: root.reactionsController ? root.reactionsController.pickerOpen : false,
      deleteConfirmOpen: root.deleteController ? root.deleteController.open : false,
      dialogOpen: root.dialogController ? root.dialogController.open : false,
      searchFocused: root.listController ? root.listController.searchFocused : false,
      composeFocused: root.composerController ? root.composerController.composeFocused : false,
      hasAttachment: root.composerController ? root.composerController.attachmentPath !== "" : false,
      replying: root.composerController ? root.composerController.replying : false,
      pane: state.pane,
      activeId: state.activeId,
      query: root.listController ? root.listController.query : "",
      unreadView: root.listController ? root.listController.unreadView : false
    };
  }

  // messageSearchTimer debounces the palette's "Messages" section so a
  // burst of keystrokes produces one conversations.list call, not one
  // per keystroke; restarted by every setPaletteQuery while typing.
  Timer {
    id: messageSearchTimer
    interval: 250
    onTriggered: root._searchMessages()
  }
}
