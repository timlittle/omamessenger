// Checks ListController against scripted services: it loads the list when
// Omarchy hands it a ready service after it was created, and a late reply
// for an earlier search does not replace the results for the query typed
// last, since the helper may answer out of order.
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
      if (method === "conversations.list") callback(null, [{ id: "c1", title: "Alex Chen", lastActivity: 2 }]);
    }
  }

  ListController {
    id: late
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
    if (late.model.count !== 1) {
      Check.fail("a service handed over after creation did not load the list");
      return;
    }

    controller.setQuery("t");
    controller.setQuery("ticket");

    service.pending["ticket"](null, [{ id: "c1", title: "Alex Chen", lastActivity: 2 }]);
    service.pending["t"](null, [{ id: "c1", title: "Alex Chen", lastActivity: 2 }, { id: "c2", title: "Mum", lastActivity: 1 }]);

    if (controller.model.count !== 1) {
      Check.fail("a late reply for an earlier search replaced the results: " + controller.model.count + " rows");
      return;
    }

    console.log("PASS ListController");
    Qt.exit(0);
  }
}
