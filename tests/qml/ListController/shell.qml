// Checks ListController against scripted services: it loads the list when
// Omarchy hands it a ready service after it was created, hides chats older
// than a month, archived with the service or hidden by the user until
// asked to show them all, dims and labels those chats once shown, orders
// pinned chats first, and a late reply for an earlier search does not
// replace the results for the query typed last, since the helper may
// answer out of order.
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

    console.log("PASS ListController");
    Qt.exit(0);
  }
}
