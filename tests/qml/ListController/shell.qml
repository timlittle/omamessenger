// Checks ListController against scripted services: it loads the list when
// Omarchy hands it a ready service after it was created, hides chats older
// than a month, archived with the service or hidden by the user until
// asked to show them all, dims and labels those chats once shown, orders
// pinned chats first, a late reply for an earlier search does not replace
// the results for the query typed last, since the helper may answer out
// of order, hiding the open conversation drops it from the standard list
// at once, moving the highlight to its neighbour and closing the pane
// rather than opening the neighbour and marking it read, and the
// all-unreads view overrides the rail filter with every unread, non-muted
// chat across every service, keeps the one just read visible while it is
// still open, and drops it the moment the user moves on to another chat.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  QtObject {
    id: service

    property string status: "starting"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", query: "", drafts: {} })
    property var pending: ({})

    signal event(string name, var data)

    // request holds each search's callback until the test answers it.
    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list" && params.query) service.pending[params.query] = callback;
    }
  }

  ListController {
    id: controller
    service: service
  }

  // readyService is already running when it reaches a controller that was
  // created without one, as happens when Omarchy recreates the service.
  QtObject {
    id: readyService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", query: "", drafts: {} })

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") callback(null, [
        { id: "c1", title: "Alex Chen", lastActivity: Date.now(), unread: 0 },
        { id: "c2", title: "Old School Friend", lastActivity: Date.now() - 400 * 24 * 3600 * 1000, unread: 0 }
      ]);
    }
  }

  ListController {
    id: late
  }

  // orderedService answers conversations.list with a mix of pinned,
  // archived and plain chats, to check ordering and visibility.
  QtObject {
    id: orderedService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", query: "", drafts: {} })

    signal event(string name, var data)

    // The helper's own conversations.list already orders pinned chats
    // first, then newest activity first; the stub mirrors that order so
    // the test exercises the controller's filtering, not its sorting.
    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") callback(null, [
        { id: "c4", title: "Pinned", lastActivity: Date.now() - 1000, unread: 0, pinned: true, archived: false, hidden: false },
        { id: "c1", title: "Plain", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false },
        { id: "c3", title: "Filed Away", lastActivity: Date.now() - 500, unread: 0, pinned: false, archived: true, hidden: false },
        { id: "c2", title: "Old", lastActivity: Date.now() - 400 * 24 * 3600 * 1000, unread: 0, pinned: false, archived: false, hidden: false },
        { id: "c5", title: "Hidden Away", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: true }
      ]);
    }
  }

  ListController {
    id: ordered
    service: orderedService
  }

  // foldService backs the fold-on-hide scenario: hiding the open
  // conversation must drop it from the standard list at once, and moving
  // the highlight onto its neighbour must never mark that neighbour read.
  QtObject {
    id: foldService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", query: "", drafts: {} })
    property var conversations: [
      { id: "f1", title: "First", lastActivity: Date.now() - 1000, unread: 0, pinned: false, archived: false, hidden: false },
      { id: "f2", title: "Second", lastActivity: Date.now() - 2000, unread: 3, pinned: false, archived: false, hidden: false },
      { id: "f3", title: "Third", lastActivity: Date.now() - 3000, unread: 0, pinned: false, archived: false, hidden: false }
    ]
    property int markReadCalls: 0

    signal event(string name, var data)

    // Every reply is a fresh copy, the way a real JSON-RPC result is:
    // handing the list controller the same object it already holds would
    // make a later mutation visible before the reply that is meant to
    // carry it, breaking the "was it visible before this change" check.
    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, foldService.conversations.map((c) => Object.assign({}, c)));
        return;
      }
      if (method === "conversations.setHidden") {
        const index = foldService.conversations.findIndex((c) => c.id === params.conversationId);
        foldService.conversations[index] = Object.assign({}, foldService.conversations[index], { hidden: params.hidden });
        callback(null, Object.assign({}, foldService.conversations[index]));
        return;
      }
      if (method === "conversations.markRead") foldService.markReadCalls++;
    }
  }

  ListController {
    id: foldList
    service: foldService

    onConversationFolded: (id) => foldConversation.closeIfOpen(id)
  }

  ConversationController {
    id: foldConversation
    service: foldService
    listController: foldList
  }

  // unreadService backs the all-unreads view scenario: chats spread across
  // both services, one muted and unread (which the view excludes, matching
  // the rail's own unread badge), and the rail filter deliberately narrowed
  // to one service throughout, since the view must override it rather than
  // narrow it further.
  QtObject {
    id: unreadService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "service:whatsapp", selectedId: "", activeId: "", query: "", drafts: {} })
    // Listed already in the order the real helper's conversations.list
    // guarantees (newest activity first), the same convention the ordered
    // service above follows, since ListController's own sort (triggered by
    // the conversation.updated event below) would otherwise reorder it out
    // from under a naively-ordered fixture.
    property var conversations: [
      { id: "u2", title: "TG Unread Newer", service: "telegram", lastActivity: Date.now(), unread: 1, muted: false, pinned: false, archived: false, hidden: false },
      { id: "u4", title: "WA Read", service: "whatsapp", lastActivity: Date.now() - 100, unread: 0, muted: false, pinned: false, archived: false, hidden: false },
      { id: "u3", title: "TG Muted Unread", service: "telegram", lastActivity: Date.now() - 500, unread: 5, muted: true, pinned: false, archived: false, hidden: false },
      { id: "u1", title: "WA Unread", service: "whatsapp", lastActivity: Date.now() - 1000, unread: 2, muted: false, pinned: false, archived: false, hidden: false }
    ]

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") callback(null, unreadService.conversations.map((c) => Object.assign({}, c)));
    }
  }

  ListController {
    id: unreadList
    service: unreadService
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run types two searches, answers the later one first, and checks the
  // earlier reply is ignored when it arrives last.
  function run(): void {
    late.service = readyService;
    if (late.model.count !== 1 || late.hiddenCount !== 1) {
      Check.fail(`a late service should load the list with the old chat hidden: ${late.model.count} shown, ${late.hiddenCount} hidden`);
      return;
    }

    late.run("list.showAll");
    if (late.model.count !== 2 || late.hiddenCount !== 0 || !readyService.uiState.showAll) {
      Check.fail("showing all chats did not show and remember them");
      return;
    }

    controller.setQuery("t");
    controller.setQuery("ticket");

    service.pending["ticket"](null, [{ id: "c1", title: "Alex Chen", lastActivity: Date.now() }]);
    service.pending["t"](null, [{ id: "c1", title: "Alex Chen", lastActivity: Date.now() }, { id: "c2", title: "Mum", lastActivity: Date.now() }]);

    if (controller.model.count !== 1) {
      Check.fail("a late reply for an earlier search replaced the results: " + controller.model.count + " rows");
      return;
    }

    const orderedIds = [];
    for (let i = 0; i < ordered.model.count; i++) orderedIds.push(ordered.model.get(i).id);
    if (JSON.stringify(orderedIds) !== '["c4","c1"]' || ordered.hiddenCount !== 3) {
      Check.fail("pinned-first ordering with archived, older and hidden chats folded away: ids "
        + JSON.stringify(orderedIds) + ", hiddenCount " + ordered.hiddenCount);
      return;
    }

    ordered.run("list.showAll");
    const afterShowIds = [];
    for (let i = 0; i < ordered.model.count; i++) afterShowIds.push(ordered.model.get(i).id);
    if (JSON.stringify(afterShowIds) !== '["c4","c1","c3","c2","c5"]' || ordered.hiddenCount !== 0 || !orderedService.uiState.showAll) {
      Check.fail("showing all chats did not reveal and remember them: ids "
        + JSON.stringify(afterShowIds) + ", hiddenCount " + ordered.hiddenCount);
      return;
    }

    const byId = {};
    for (let i = 0; i < ordered.model.count; i++) { const row = ordered.model.get(i); byId[row.id] = row; }
    if (byId.c1.dimmed || byId.c1.dimLabel !== "") {
      Check.fail("a plain chat shown in the standard list is dimmed in show-all: " + JSON.stringify(byId.c1));
      return;
    }
    if (!byId.c3.dimmed || byId.c3.dimLabel !== "Archived") {
      Check.fail("an archived chat is not dimmed with an \"Archived\" label: " + JSON.stringify(byId.c3));
      return;
    }
    if (!byId.c2.dimmed || byId.c2.dimLabel !== "") {
      Check.fail("a merely older chat is not dimmed, or wrongly carries a label: " + JSON.stringify(byId.c2));
      return;
    }
    if (!byId.c5.dimmed || byId.c5.dimLabel !== "Hidden") {
      Check.fail("a hidden chat is not dimmed with a \"Hidden\" label: " + JSON.stringify(byId.c5));
      return;
    }

    ordered.run("list.showAll");
    if (ordered.model.count !== 2 || orderedService.uiState.showAll) {
      Check.fail("toggling show-all again did not fold the chats back away");
      return;
    }

    root.checkFoldOnHide();
  }

  // checkFoldOnHide opens the second of three chats, hides it, and checks
  // it disappears from the standard list immediately, the highlight moves
  // to its neighbour, the conversation pane closes rather than opening
  // the neighbour, and no mark-read ever fires beyond the original open.
  function checkFoldOnHide(): void {
    foldConversation.open(foldList.findConversation("f2"));
    if (foldConversation.activeId !== "f2" || foldList.selectedId !== "f2") {
      Check.fail("opening the second chat did not make it the open and selected one");
      return;
    }
    if (foldService.markReadCalls !== 1) {
      Check.fail("opening an unread chat should mark it read exactly once, got " + foldService.markReadCalls);
      return;
    }

    foldList.run("chat.hide");

    const ids = [];
    for (let i = 0; i < foldList.model.count; i++) ids.push(foldList.model.get(i).id);
    if (JSON.stringify(ids) !== '["f1","f3"]') {
      Check.fail("hiding the open chat did not drop it from the standard list at once: " + JSON.stringify(ids));
      return;
    }
    if (foldList.selectedId !== "f3") {
      Check.fail("hiding the open chat did not move the highlight to its neighbour: selected " + foldList.selectedId);
      return;
    }
    if (foldConversation.activeId !== "" || foldConversation.pane !== "list") {
      Check.fail("hiding the open chat did not close the conversation pane: activeId "
        + foldConversation.activeId + " pane " + foldConversation.pane);
      return;
    }
    if (foldService.markReadCalls !== 1) {
      Check.fail("hiding the open chat marked a neighbour read: markReadCalls " + foldService.markReadCalls);
      return;
    }

    // f3 is now the last row; hiding it should fall back to the one
    // before it instead of a next row that no longer exists.
    foldList.run("chat.hide");
    const idsAfter = [];
    for (let i = 0; i < foldList.model.count; i++) idsAfter.push(foldList.model.get(i).id);
    if (JSON.stringify(idsAfter) !== '["f1"]' || foldList.selectedId !== "f1") {
      Check.fail("hiding the last row did not fall back to the previous one: ids "
        + JSON.stringify(idsAfter) + ", selected " + foldList.selectedId);
      return;
    }

    root.checkUnreadView();
  }

  // checkUnreadView drives the all-unreads view against unreadList: it
  // overrides the rail filter (left narrowed to "service:whatsapp" the
  // whole time) rather than narrowing it further, excludes a muted chat
  // the same way the rail's own unread badge does, orders by most recent
  // activity, keeps the chat just read visible while it is still the one
  // open, drops it the moment the user moves on to another, and leaves the
  // view exactly where the rail filter had been.
  function checkUnreadView(): void {
    const idsBefore = [];
    for (let i = 0; i < unreadList.model.count; i++) idsBefore.push(unreadList.model.get(i).id);
    if (JSON.stringify(idsBefore) !== '["u4","u1"]') {
      Check.fail("setup: the whatsapp rail filter should show u4 and u1 before the unread view is on, got "
        + JSON.stringify(idsBefore));
      return;
    }

    unreadList.run("list.unread");
    const unreadIds = [];
    for (let i = 0; i < unreadList.model.count; i++) unreadIds.push(unreadList.model.get(i).id);
    if (JSON.stringify(unreadIds) !== '["u2","u1"]') {
      Check.fail("the unread view did not override the rail filter with every unread, non-muted chat newest first: "
        + JSON.stringify(unreadIds));
      return;
    }

    // Opening u2 from the view and reading it (its unread count drops to
    // zero) must not yank it out from under the cursor while it is still
    // the one open.
    unreadService.uiState = Object.assign({}, unreadService.uiState, { activeId: "u2" });
    const u2 = unreadService.conversations.find((c) => c.id === "u2");
    unreadService.event("conversation.updated", Object.assign({}, u2, { unread: 0 }));
    const afterRead = [];
    for (let i = 0; i < unreadList.model.count; i++) afterRead.push(unreadList.model.get(i).id);
    if (JSON.stringify(afterRead) !== '["u2","u1"]') {
      Check.fail("reading the open chat dropped it from the unread view before it was left: " + JSON.stringify(afterRead));
      return;
    }

    // Moving on to a different chat drops the now-read one at once.
    unreadService.uiState = Object.assign({}, unreadService.uiState, { activeId: "u1" });
    unreadService.event("unread.changed", {});
    const afterMoveOn = [];
    for (let i = 0; i < unreadList.model.count; i++) afterMoveOn.push(unreadList.model.get(i).id);
    if (JSON.stringify(afterMoveOn) !== '["u1"]') {
      Check.fail("moving on to another chat did not drop the now-read one from the unread view: " + JSON.stringify(afterMoveOn));
      return;
    }

    unreadList.run("list.unread");
    const afterToggleOff = [];
    for (let i = 0; i < unreadList.model.count; i++) afterToggleOff.push(unreadList.model.get(i).id);
    if (JSON.stringify(afterToggleOff) !== '["u4","u1"]') {
      Check.fail("toggling the unread view off did not return to the previous rail-filtered list: " + JSON.stringify(afterToggleOff));
      return;
    }

    console.log("PASS ListController");
    Qt.exit(0);
  }
}
