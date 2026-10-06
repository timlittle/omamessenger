// Checks DialogController against a scripted service that answers contact
// searches out of order, as the helper may: a late reply for an earlier
// query must not replace the contacts for the query typed last.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  QtObject {
    id: service

    property var accounts: [{ id: "a1", service: "telegram", name: "Telegram" }]
    property var pending: ({})

    // request holds each contact search's callback until the test answers it.
    function request(method: string, params: var, callback: var): void {
      if (method === "contacts.list") service.pending[params.query] = callback;
    }
  }

  DialogController {
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
    controller.run("chat.new");
    controller.setQuery("b");
    controller.setQuery("ben");

    service.pending["ben"](null, [{ accountId: "a1", remoteId: "r1", name: "Ben Okafor" }]);
    service.pending["b"](null, [{ accountId: "a1", remoteId: "r1", name: "Ben Okafor" }, { accountId: "a1", remoteId: "r2", name: "Bea" }]);
    service.pending[""](null, []);

    if (controller.contacts.length !== 1) {
      Check.fail("a late reply for an earlier search replaced the contacts: " + controller.contacts.length);
      return;
    }

    console.log("PASS DialogController");
    Qt.exit(0);
  }
}
