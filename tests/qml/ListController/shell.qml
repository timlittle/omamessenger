// Checks ListController against scripted services: it loads the list when
// Omarchy hands it a ready service after it was created, hides chats older
// than a month, archived with the service or hidden by the user until
// asked to show them all, dims and labels those chats once shown, orders
// pinned chats first, a late reply for an earlier search does not replace
// the results for the query typed last, since the helper may answer out
// of order, hiding the open conversation drops it from the standard list
// at once, moving the highlight to its neighbour and closing the pane
// rather than opening the neighbour and marking it read, the all-unreads
// view overrides the rail filter with every unread, non-muted chat across
// every service, keeps the one just read visible while it is still open,
// and drops it the moment the user moves on to another chat, archiving a
// chat asks first and marks it read, archiving every read chat asks before
// doing it and skips pinned or unread ones, snoozing a chat hides it until
// its reminder comes due, when it returns to the top marked "Reminder",
// and setting or resetting a chat's own notification sound reaches
// conversations.setNotificationSound or .resetNotificationSound with
// the selected conversation, and blocking, unblocking or reporting a
// contact as spam each raise their own toast notice naming what
// happened, WhatsApp's own spam report reading as a plain block since
// it has no report of its own.
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

  // archiveService backs the archive scenarios: a plain read chat for
  // the single "archive and mark read" shortcut, a pinned read chat and
  // an unread chat that "archive all read" must both leave alone, and a
  // second plain read chat for the bulk question itself.
  QtObject {
    id: archiveService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", activeId: "", query: "", drafts: {} })
    property var conversations: [
      { id: "a1", title: "Plain Read", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false },
      { id: "a2", title: "Pinned Read", lastActivity: Date.now(), unread: 0, pinned: true, archived: false, hidden: false },
      { id: "a3", title: "Unread", lastActivity: Date.now(), unread: 2, pinned: false, archived: false, hidden: false },
      { id: "a4", title: "Plain Read Two", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false }
    ]
    property int markReadCalls: 0
    property var archivedIds: []

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, archiveService.conversations.map((c) => Object.assign({}, c)));
        return;
      }
      if (method === "conversations.markRead") {
        archiveService.markReadCalls++;
        callback(null, {});
        return;
      }
      if (method === "conversations.setArchived") {
        const index = archiveService.conversations.findIndex((c) => c.id === params.conversationId);
        archiveService.conversations[index] = Object.assign({}, archiveService.conversations[index], { archived: params.archived });
        archiveService.archivedIds.push(params.conversationId);
        callback(null, Object.assign({}, archiveService.conversations[index]));
      }
    }
  }

  ListController {
    id: archiveList
    service: archiveService
  }

  // snoozeService backs the snooze scenario: one chat, snoozed and then
  // brought back due by a conversation.updated event, the way the
  // helper's own reminder scheduler reports it.
  QtObject {
    id: snoozeService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", activeId: "", query: "", drafts: {} })
    property var conversations: [
      { id: "s1", title: "Snooze Me", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false, reminderAt: 0 }
    ]

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, snoozeService.conversations.map((c) => Object.assign({}, c)));
        return;
      }
      if (method === "conversations.setReminder") {
        const index = snoozeService.conversations.findIndex((c) => c.id === params.conversationId);
        snoozeService.conversations[index] = Object.assign({}, snoozeService.conversations[index], { reminderAt: params.at ?? 0 });
        callback(null, Object.assign({}, snoozeService.conversations[index]));
      }
    }
  }

  ListController {
    id: snoozeList
    service: snoozeService
  }

  // blockService backs the block, unblock and report-spam scenarios: one
  // direct chat, not yet blocked.
  QtObject {
    id: blockService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", activeId: "", query: "", drafts: {} })
    property var conversations: [
      { id: "b1", title: "Spammer", service: "telegram", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false, blocked: false }
    ]
    property var blockedIds: []
    property int spamReports: 0

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, blockService.conversations.map((c) => Object.assign({}, c)));
        return;
      }
      if (method === "contacts.block" || method === "contacts.unblock") {
        const blocked = method === "contacts.block";
        const index = blockService.conversations.findIndex((c) => c.id === params.conversationId);
        blockService.conversations[index] = Object.assign({}, blockService.conversations[index], { blocked });
        blockService.blockedIds.push(params.conversationId + " " + blocked);
        callback(null, Object.assign({}, blockService.conversations[index]));
        return;
      }
      if (method === "conversations.reportSpam") {
        blockService.spamReports++;
        const index = blockService.conversations.findIndex((c) => c.id === params.conversationId);
        blockService.conversations[index] = Object.assign({}, blockService.conversations[index], { blocked: true, hidden: true });
        callback(null, Object.assign({}, blockService.conversations[index]));
      }
    }
  }

  ListController {
    id: blockList
    service: blockService
  }

  // waSpamService backs the one WhatsApp-specific scenario in
  // checkWhatsAppSpamNotice: a direct chat whose service is
  // "whatsapp", not yet blocked.
  QtObject {
    id: waSpamService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", activeId: "", query: "", drafts: {} })
    property var conversations: [
      { id: "wa1", title: "WhatsApp Spammer", service: "whatsapp", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false, blocked: false }
    ]

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, waSpamService.conversations.map((c) => Object.assign({}, c)));
        return;
      }
      if (method === "conversations.reportSpam") {
        const index = waSpamService.conversations.findIndex((c) => c.id === params.conversationId);
        waSpamService.conversations[index] = Object.assign({}, waSpamService.conversations[index], { blocked: true, hidden: true });
        callback(null, Object.assign({}, waSpamService.conversations[index]));
      }
    }
  }

  ListController {
    id: waSpamList
    service: waSpamService
  }

  // pinService backs the pin-reordering scenario: three pinned chats and
  // one plain one, to check chat.movePinUp/chat.movePinDown and
  // movePinTo (the drag handler's own entry point) each reach
  // conversations.movePin with the right arguments, the detailed
  // reordering itself already being proven exhaustively by the pure
  // ui/lib/Pin.js tests and the backend's own store and app tests.
  QtObject {
    id: pinService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", activeId: "", query: "", drafts: {} })
    property var conversations: [
      { id: "p1", title: "First Pinned", lastActivity: Date.now(), unread: 0, pinned: true, pinOrder: 1, archived: false, hidden: false },
      { id: "p2", title: "Second Pinned", lastActivity: Date.now(), unread: 0, pinned: true, pinOrder: 2, archived: false, hidden: false },
      { id: "p3", title: "Third Pinned", lastActivity: Date.now(), unread: 0, pinned: true, pinOrder: 3, archived: false, hidden: false },
      { id: "p4", title: "Not Pinned", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false }
    ]
    property var moveCalls: []

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, pinService.conversations.map((c) => Object.assign({}, c)));
        return;
      }
      if (method === "conversations.movePin") {
        pinService.moveCalls.push(params.conversationId + " " + params.direction);
        const conversation = pinService.conversations.find((c) => c.id === params.conversationId);
        callback(null, Object.assign({}, conversation));
      }
    }
  }

  ListController {
    id: pinList
    service: pinService
  }

  // soundService backs the notification sound scenario: two chats
  // with no custom sound of their own yet. The real file-picking
  // dialog is never driven here (see ui-testing.md): this checks only
  // the wiring either side of it. filePickCalls records every
  // files.pick request's own params, for confirming
  // chat.setNotificationSound asks for one before ever touching
  // conversations.setNotificationSound; filePickPath, left null by
  // default, answers it with a path once a test sets one, the same
  // way a picked file's own accept handler would
  // (notificationSoundPicked, called directly for "n1" below, the
  // same way other tests here call a controller's own private
  // functions, to check conversations.setNotificationSound's own
  // params without going through files.pick at all). The file's own
  // validity is the helper's job, covered by the backend's own tests.
  QtObject {
    id: soundService

    property string status: "ready"
    property var accounts: []
    property var uiState: ({ railKey: "all", selectedId: "", activeId: "", query: "", drafts: {} })
    property var conversations: [
      { id: "n1", title: "Needs A Sound", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false, notificationSound: "" },
      { id: "n2", title: "Also Needs A Sound", lastActivity: Date.now(), unread: 0, pinned: false, archived: false, hidden: false, notificationSound: "" }
    ]
    property var setCalls: []
    property int resetCalls: 0
    property var filePickCalls: []
    property var filePickPath: null

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        callback(null, soundService.conversations.map((c) => Object.assign({}, c)));
        return;
      }
      if (method === "files.pick") {
        soundService.filePickCalls.push({ purpose: params.purpose, title: params.title });
        if (soundService.filePickPath !== null) callback(null, { path: soundService.filePickPath });
        return;
      }
      if (method === "conversations.setNotificationSound") {
        soundService.setCalls.push(params.conversationId + " " + params.path);
        const index = soundService.conversations.findIndex((c) => c.id === params.conversationId);
        soundService.conversations[index] = Object.assign({}, soundService.conversations[index], { notificationSound: "custom" });
        callback(null, Object.assign({}, soundService.conversations[index]));
        return;
      }
      if (method === "conversations.resetNotificationSound") {
        soundService.resetCalls++;
        const index = soundService.conversations.findIndex((c) => c.id === params.conversationId);
        soundService.conversations[index] = Object.assign({}, soundService.conversations[index], { notificationSound: "" });
        callback(null, Object.assign({}, soundService.conversations[index]));
      }
    }
  }

  ListController {
    id: soundList
    service: soundService
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

    root.checkArchive();
  }

  // checkArchive drives the single "archive and mark read" shortcut on
  // one chat, then the bulk "archive all read" question, which must
  // offer only the remaining plain read chat (never the pinned or the
  // unread one), can be cancelled without archiving anything, and, once
  // confirmed, archives exactly that one.
  function checkArchive(): void {
    archiveList.selectId("a1");
    archiveList.run("chat.archiveRead");

    if (!archiveList.archiveOneOpen || archiveList.archiveOneTitle !== "Plain Read") {
      Check.fail("chat.archiveRead did not ask before archiving: open " + archiveList.archiveOneOpen
        + ", title " + archiveList.archiveOneTitle);
      return;
    }

    archiveList.run("archiveAll.cancel");
    if (archiveList.archiveOneOpen || archiveService.archivedIds.length !== 0) {
      Check.fail("cancelling the single archive question must not mark read or archive anything");
      return;
    }

    archiveList.run("chat.archiveRead");
    archiveList.run("archiveAll.accept");

    if (archiveService.markReadCalls !== 1 || JSON.stringify(archiveService.archivedIds) !== '["a1"]') {
      Check.fail("confirming chat.archiveRead did not mark read then archive the selected chat: markReadCalls "
        + archiveService.markReadCalls + ", archivedIds " + JSON.stringify(archiveService.archivedIds));
      return;
    }

    // "u" undoes the archive through conversations.setArchived with
    // archived false, the same call unarchiving directly would make.
    archiveList.run("chat.undo");
    if (JSON.stringify(archiveService.archivedIds) !== '["a1","a1"]' || archiveList.findConversation("a1").archived) {
      Check.fail("chat.undo after an archive did not unarchive a1: archivedIds " + JSON.stringify(archiveService.archivedIds));
      return;
    }

    // Archiving it again, resetting the counters above, so the rest of
    // this scenario can continue exactly as it did before undo existed.
    // The first archive already moved the cursor off a1, onto its
    // neighbour, the moment it dropped out of the standard list (see
    // _foldIfNeeded), so the cursor is put back on it explicitly.
    archiveService.archivedIds = [];
    archiveService.markReadCalls = 0;
    archiveList.selectId("a1");
    archiveList.run("chat.archiveRead");
    archiveList.run("archiveAll.accept");

    const idsAfterSingle = [];
    for (let i = 0; i < archiveList.model.count; i++) idsAfterSingle.push(archiveList.model.get(i).id);
    if (idsAfterSingle.includes("a1")) {
      Check.fail("the archived chat is still in the standard list: " + JSON.stringify(idsAfterSingle));
      return;
    }

    archiveList.run("list.archiveAllRead");
    if (!archiveList.archiveAllOpen || archiveList.archiveAllCount !== 1) {
      Check.fail("archive-all should offer only the one remaining plain read chat: open "
        + archiveList.archiveAllOpen + ", count " + archiveList.archiveAllCount);
      return;
    }

    archiveList.run("archiveAll.cancel");
    if (archiveList.archiveAllOpen || archiveService.archivedIds.includes("a4")) {
      Check.fail("cancelling the archive-all question must not archive anything");
      return;
    }

    archiveList.run("list.archiveAllRead");
    archiveList.run("archiveAll.accept");

    if (archiveList.archiveAllOpen || JSON.stringify(archiveService.archivedIds) !== '["a1","a4"]') {
      Check.fail("confirming archive-all did not archive exactly the offered chat: archivedIds "
        + JSON.stringify(archiveService.archivedIds));
      return;
    }

    const idsAfterBulk = [];
    for (let i = 0; i < archiveList.model.count; i++) idsAfterBulk.push(archiveList.model.get(i).id);
    if (JSON.stringify(idsAfterBulk) !== '["a2","a3"]') {
      Check.fail("the pinned and unread chats should remain, nothing else: " + JSON.stringify(idsAfterBulk));
      return;
    }

    root.checkArchiveReadTogglesAnAlreadyArchivedChat();
  }

  // checkArchiveReadTogglesAnAlreadyArchivedChat confirms Shift+A
  // (chat.archiveRead) unarchives a1 - already archived by the bulk
  // question above - at once, with no question and no extra mark-read
  // call, the same as the command palette's own "Archive or unarchive
  // chat" toggle: archiving always asks, but a1 is already on the other
  // side of that question by now.
  function checkArchiveReadTogglesAnAlreadyArchivedChat(): void {
    archiveService.markReadCalls = 0;
    const archivedIdsBefore = archiveService.archivedIds.length;
    archiveList.selectId("a1");
    archiveList.run("chat.archiveRead");

    if (archiveList.archiveOneOpen) {
      Check.fail("chat.archiveRead asked before unarchiving an already-archived chat");
      return;
    }
    if (archiveList.findConversation("a1").archived) {
      Check.fail("chat.archiveRead did not unarchive the already-archived chat");
      return;
    }
    if (archiveService.markReadCalls !== 0) {
      Check.fail("unarchiving should not mark the chat read: markReadCalls " + archiveService.markReadCalls);
      return;
    }
    if (archiveService.archivedIds.length !== archivedIdsBefore + 1) {
      Check.fail("unarchiving should make exactly one conversations.setArchived call");
      return;
    }

    root.checkSnooze();
  }

  // checkSnooze snoozes a chat (hiding it from the standard list, like
  // hide), brings it back due the way the helper's own reminder
  // scheduler does (a conversation.updated event with reminderAt now in
  // the past), and checks it returns to the top marked "Reminder", then
  // that unsnoozing clears the mark.
  function checkSnooze(): void {
    snoozeList.selectId("s1");
    snoozeList.run("chat.snoozeLaterToday");

    const idsSnoozed = [];
    for (let i = 0; i < snoozeList.model.count; i++) idsSnoozed.push(snoozeList.model.get(i).id);
    if (idsSnoozed.includes("s1")) {
      Check.fail("snoozing the chat did not hide it from the standard list: " + JSON.stringify(idsSnoozed));
      return;
    }

    const snoozed = snoozeService.conversations.find((c) => c.id === "s1");
    snoozeService.event("conversation.updated", Object.assign({}, snoozed, { reminderAt: Date.now() - 1000 }));

    const idsDue = [];
    const rowsById = {};
    for (let i = 0; i < snoozeList.model.count; i++) {
      const row = snoozeList.model.get(i);
      idsDue.push(row.id);
      rowsById[row.id] = row;
    }
    if (idsDue[0] !== "s1" || !rowsById.s1.reminderDue) {
      Check.fail("a due reminder did not return to the top of the list marked: ids "
        + JSON.stringify(idsDue) + ", row " + JSON.stringify(rowsById.s1));
      return;
    }

    snoozeList.selectId("s1"); // it fell off the cursor when it was the only, now-hidden row
    snoozeList.run("chat.unsnooze");
    if (snoozeList.model.count !== 1 || snoozeList.model.get(0).reminderDue) {
      Check.fail("unsnoozing did not clear the reminder mark");
      return;
    }

    root.checkBlock();
  }

  // checkBlock drives blocking a contact (which asks for confirmation
  // and can be cancelled without blocking anything), unblocking (which
  // needs none), and reporting as spam (which also asks, and on
  // confirming blocks the contact and hides the chat).
  function checkBlock(): void {
    blockList.selectId("b1");
    blockList.run("chat.block");

    if (!blockList.blockConfirmOpen || blockList.blockConfirmKind !== "block") {
      Check.fail("chat.block did not open the block confirmation: open " + blockList.blockConfirmOpen + ", kind " + blockList.blockConfirmKind);
      return;
    }

    blockList.run("blockConfirm.cancel");
    if (blockList.blockConfirmOpen || blockService.blockedIds.length !== 0) {
      Check.fail("cancelling the block question must not block anything");
      return;
    }

    blockList.run("chat.block");
    blockList.run("blockConfirm.accept");

    if (blockList.blockConfirmOpen || JSON.stringify(blockService.blockedIds) !== '["b1 true"]') {
      Check.fail("confirming chat.block did not block exactly the selected contact: blockedIds "
        + JSON.stringify(blockService.blockedIds));
      return;
    }
    if (!blockList.model.get(0).blocked) {
      Check.fail("the blocked chat's own row was not updated");
      return;
    }
    if (blockList.notice !== "Blocked Spammer · u to undo") {
      Check.fail("confirming chat.block left notice \"" + blockList.notice + "\", want the undo notice");
      return;
    }

    // "u" undoes the block exactly through contacts.unblock, the same
    // call chat.unblock itself makes.
    blockList.run("chat.undo");
    if (JSON.stringify(blockService.blockedIds) !== '["b1 true","b1 false"]' || blockList.model.get(0).blocked) {
      Check.fail("chat.undo after a block did not unblock through contacts.unblock: blockedIds "
        + JSON.stringify(blockService.blockedIds));
      return;
    }

    // Blocking again, so the rest of the scenario can drive chat.unblock
    // itself the way it always has.
    blockList.run("chat.block");
    blockList.run("blockConfirm.accept");
    blockList.clearNotice();

    blockList.run("chat.unblock");
    if (JSON.stringify(blockService.blockedIds) !== '["b1 true","b1 false","b1 true","b1 false"]' || blockList.model.get(0).blocked) {
      Check.fail("chat.unblock did not unblock at once, with no confirmation needed: blockedIds "
        + JSON.stringify(blockService.blockedIds));
      return;
    }
    if (blockList.notice !== "Unblocked") {
      Check.fail("chat.unblock left notice \"" + blockList.notice + "\", want \"Unblocked\"");
      return;
    }

    blockList.run("chat.reportSpam");
    if (!blockList.blockConfirmOpen || blockList.blockConfirmKind !== "spam") {
      Check.fail("chat.reportSpam did not open the spam confirmation: open " + blockList.blockConfirmOpen + ", kind " + blockList.blockConfirmKind);
      return;
    }

    blockList.run("blockConfirm.accept");
    if (blockList.blockConfirmOpen || blockService.spamReports !== 1) {
      Check.fail("confirming chat.reportSpam did not report the contact as spam");
      return;
    }
    if (blockList.model.count !== 0) {
      Check.fail("reporting spam did not hide the conversation from the standard list: " + blockList.model.count);
      return;
    }
    if (blockList.notice !== "Reported as spam and blocked") {
      Check.fail("confirming chat.reportSpam on a Telegram chat left notice \"" + blockList.notice + "\", want \"Reported as spam and blocked\"");
      return;
    }

    root.checkWhatsAppSpamNotice();
  }

  // checkWhatsAppSpamNotice confirms a spam report on a WhatsApp chat
  // raises the plainer "Blocked" notice instead: WhatsApp's own API has
  // no report of its own (see whatsapp/block.go), so this only ever
  // blocks there, and a "Reported as spam" toast would promise
  // something that did not happen.
  function checkWhatsAppSpamNotice(): void {
    waSpamList.selectId("wa1");
    waSpamList.run("chat.reportSpam");
    waSpamList.run("blockConfirm.accept");

    if (waSpamList.notice !== "Blocked") {
      Check.fail("confirming chat.reportSpam on a WhatsApp chat left notice \"" + waSpamList.notice + "\", want \"Blocked\"");
      return;
    }

    root.checkMovePin();
  }

  // checkMovePin drives chat.movePinUp/chat.movePinDown on a pinned and
  // on a not-pinned chat, then movePinTo, the entry point a pinned
  // row's drag handler calls once it drops - see
  // ConversationList.qml's own DragHandler and ListColumn.qml's
  // pinReordered, which MessengerLayout wires straight to it.
  function checkMovePin(): void {
    pinList.selectId("p2");
    pinList.run("chat.movePinUp");
    if (JSON.stringify(pinService.moveCalls) !== '["p2 up"]') {
      Check.fail("chat.movePinUp on a pinned chat did not call conversations.movePin(up): " + JSON.stringify(pinService.moveCalls));
      return;
    }

    pinService.moveCalls = [];
    pinList.selectId("p4");
    pinList.run("chat.movePinDown");
    if (pinService.moveCalls.length !== 0) {
      Check.fail("chat.movePinDown on a chat that is not pinned should call nothing: " + JSON.stringify(pinService.moveCalls));
      return;
    }

    // p1, p2, p3 is the pinned order conversations.list reported; moving
    // p1 down to index 2 takes two adjacent swaps, each asking the
    // helper to move it one more step down.
    pinService.moveCalls = [];
    pinList.movePinTo("p1", 2);
    if (JSON.stringify(pinService.moveCalls) !== '["p1 down","p1 down"]') {
      Check.fail("movePinTo(p1, 2) did not chain two downward moves: " + JSON.stringify(pinService.moveCalls));
      return;
    }

    // Moving p3 up to index 0 takes two steps the other way.
    pinService.moveCalls = [];
    pinList.movePinTo("p3", 0);
    if (JSON.stringify(pinService.moveCalls) !== '["p3 up","p3 up"]') {
      Check.fail("movePinTo(p3, 0) did not chain two upward moves: " + JSON.stringify(pinService.moveCalls));
      return;
    }

    // Dropped back on its own place, nothing is asked for at all.
    pinService.moveCalls = [];
    pinList.movePinTo("p2", 1);
    if (pinService.moveCalls.length !== 0) {
      Check.fail("movePinTo back to its own place should call nothing: " + JSON.stringify(pinService.moveCalls));
      return;
    }

    root.checkNotificationSound();
  }

  // checkFilePickWiring confirms chat.setNotificationSound asks the
  // helper to open a sound-purpose files.pick before ever touching
  // conversations.setNotificationSound, and that a successful pick
  // flows straight through to it with the right conversation and the
  // picked path - the portal round trip itself, never driven as a
  // real dialog here (see ui-testing.md). Uses "n2" so it leaves "n1"
  // untouched for checkNotificationSound's own, separate check of
  // notificationSoundPicked's own wiring.
  function checkFilePickWiring(): bool {
    soundList.selectId("n2");
    soundService.filePickPath = "/tmp/portal-pick.ogg";
    soundList.run("chat.setNotificationSound");

    if (soundService.filePickCalls.length !== 1 || soundService.filePickCalls[0].purpose !== "sound") {
      return Check.fail("chat.setNotificationSound did not ask for a sound-purpose file pick: " + JSON.stringify(soundService.filePickCalls));
    }
    if (JSON.stringify(soundService.setCalls) !== '["n2 /tmp/portal-pick.ogg"]') {
      return Check.fail("a successful portal pick did not reach conversations.setNotificationSound with the picked path: "
        + JSON.stringify(soundService.setCalls));
    }

    soundService.setCalls = [];
    soundService.filePickCalls = [];
    soundService.filePickPath = null;
    return true;
  }

  // checkNotificationSound drives chat.setNotificationSound (captures
  // the current conversation, then a picked path reaching
  // conversations.setNotificationSound with it once accepted) and
  // chat.resetNotificationSound (conversations.resetNotificationSound
  // on whichever is current). checkFilePickWiring, run first, confirms
  // the portal round trip itself: chat.setNotificationSound asks for
  // a sound-purpose files.pick before touching
  // conversations.setNotificationSound at all, and a successful pick
  // flows straight through to it with the picked path - never opening
  // a real dialog (see ui-testing.md).
  function checkNotificationSound(): void {
    if (!root.checkFilePickWiring()) return;

    soundList.selectId("n1");
    soundList.run("chat.setNotificationSound");
    soundList.notificationSoundPicked("/tmp/ping.ogg");

    if (JSON.stringify(soundService.setCalls) !== '["n1 /tmp/ping.ogg"]') {
      Check.fail("picking a file did not call conversations.setNotificationSound with the selected chat and the picked path: "
        + JSON.stringify(soundService.setCalls));
      return;
    }
    if (soundList.findConversation("n1").notificationSound !== "custom") {
      Check.fail("the row was not updated with the helper's own reply");
      return;
    }

    soundList.run("chat.resetNotificationSound");
    if (soundService.resetCalls !== 1 || soundList.findConversation("n1").notificationSound !== "") {
      Check.fail("chat.resetNotificationSound did not clear the conversation's own sound: resetCalls "
        + soundService.resetCalls + ", row " + JSON.stringify(soundList.findConversation("n1")));
      return;
    }

    root.checkTypingGuard();
  }

  // checkTypingGuard confirms guardTyping's own wiring: a second plain
  // action right on the heels of the first is suppressed and raises the
  // "press i to write" toast, while a navigation action immediately
  // afterwards is never suppressed. The decision itself (the burst
  // window, which actions count as navigation) is proven exhaustively
  // in tests/unit/typingguard.test.cjs; this only checks the key
  // reaching guardTyping at all.
  function checkTypingGuard(): void {
    if (archiveList.guardTyping("chat.mute")) {
      Check.fail("the first plain action of a burst must never be suppressed");
      return;
    }

    if (!archiveList.guardTyping("chat.pin")) {
      Check.fail("a second plain action right after the first must be suppressed as a likely mistype");
      return;
    }
    if (!archiveList.notice.startsWith("Press ") || !archiveList.notice.endsWith("to write a message")) {
      Check.fail("a suppressed burst must show the write-a-message toast, got \"" + archiveList.notice + "\"");
      return;
    }

    if (archiveList.guardTyping("cursor.down")) {
      Check.fail("navigation must never be suppressed, even right after a burst");
      return;
    }

    // Opening something or switching into writing mode is never held
    // back, even right on the heels of a guarded action: this is what
    // distinguishes "mistyped burst" from "someone chaining real
    // shortcuts fast" (see docs/decisions.md).
    archiveList.guardTyping("chat.mute");
    if (archiveList.guardTyping("compose.focus")) {
      Check.fail("a mode switch must never be suppressed, even right after a guarded action");
      return;
    }

    // An unmatched key - the shape an ordinary typed letter takes once
    // it hits a context where it is not bound to anything - still
    // counts towards the next burst, even though it never does
    // anything itself.
    archiveList.guardTyping("");
    if (!archiveList.guardTyping("chat.mute")) {
      Check.fail("a guarded action right after an unmatched key must still be suppressed");
      return;
    }

    console.log("PASS ListController");
    Qt.exit(0);
  }
}
