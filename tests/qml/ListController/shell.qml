// Checks ListController against a scripted service that answers searches
// out of order, as the helper may: a late reply for an earlier query must
// not replace the results for the query the user typed last.
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

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run types two searches, answers the later one first, and checks the
  // earlier reply is ignored when it arrives last.
  function run(): void {
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
