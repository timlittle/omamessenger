// Checks that ConversationController tells the helper again which
// conversation is open and whether the window is active once the
// helper's status comes back to "ready": a helper that crashed and
// restarted starts over with nobody's focus recorded (see Ingest.Looked
// in backend/internal/app/ingest.go), so without this, a chat the user
// never closed stays wrongly treated as unwatched after the restart.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  // fakeService records every ui.setFocus call's params and lets the
  // test flip status to simulate the helper restarting.
  QtObject {
    id: fakeService

    property string status: "ready"
    property var focusCalls: []
    property var uiState: ({})
    property var accounts: []

    // Only ui.setFocus is recorded; every other request (loading
    // messages, marking read, …) is left unanswered, since this test
    // only cares whether the controller tells the helper who is
    // looking again once it comes back.
    function request(method: string, params: var, callback: var): void {
      if (method === "ui.setFocus") fakeService.focusCalls.push(params);
    }
  }

  ConversationController {
    id: conversationController
    service: fakeService
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run opens a conversation, then simulates the helper crashing and
  // coming back, and checks the controller told it again who is looking.
  function run(): void {
    conversationController.open({ id: "chat-1" });
    conversationController.setWindowActive(true);
    fakeService.focusCalls = [];

    fakeService.status = "starting";
    fakeService.status = "ready";

    if (fakeService.focusCalls.length === 0) {
      Check.fail("ui.setFocus was not resent once the helper came back to ready");
      return;
    }

    const last = fakeService.focusCalls[fakeService.focusCalls.length - 1];
    if (last.conversationId !== "chat-1" || last.windowActive !== true) {
      Check.fail("ui.setFocus after restart carried " + JSON.stringify(last) + ", want chat-1/true");
      return;
    }

    console.log("PASS ConversationController");
    Qt.exit(0);
  }
}
