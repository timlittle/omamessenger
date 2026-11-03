// Checks that ConversationController tells the helper again which
// conversation is open and whether the window is active once the
// helper's status comes back to "ready": a helper that crashed and
// restarted starts over with nobody's focus recorded (see Ingest.Looked
// in backend/internal/app/ingest.go), so without this, a chat the user
// never closed stays wrongly treated as unwatched after the restart.
// Also checks the wiring between a message flood and the highlighted
// message: applyMessage moves the highlight off a row the capped
// timeline just evicted, rather than leaving it pointing at a message
// that is no longer loaded (see MessageTimeline.upsert and
// tests/unit/timeline.test.cjs for the capping decision itself). Also
// checks that a typing event's own name is marked with "~" when it is
// not verified, the same as everywhere else a self-chosen name can
// reach the UI (see docs/decisions.md).
import QtQuick
import Quickshell
import "ui/controllers"
import "ui/lib/Timeline.js" as Timeline
import "Check.js" as Check

ShellRoot {
  id: root

  // fakeService records every ui.setFocus call's params, emits event()
  // the way the real service does for a typing notification, and lets
  // the test flip status to simulate the helper restarting.
  QtObject {
    id: fakeService

    property string status: "ready"
    property var focusCalls: []
    property var uiState: ({})
    property var accounts: []

    // event mirrors the real service's own signal: ConversationController
    // listens to it with a Connections block the same way it would for
    // the real helper, so the test can simulate a typing notification
    // just by emitting it directly.
    signal event(string name, var data)

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

    if (!root.checkFloodMovesHighlightOffAnEvictedRow()) return;
    if (!root.checkTypingMarksAnUnverifiedName()) return;

    console.log("PASS ConversationController");
    Qt.exit(0);
  }

  // message is a minimal message with id and created time, for chat-1.
  function message(id: string, created: int): var {
    return { id: id, conversationId: "chat-1", senderId: "s", senderName: "S", text: id, outgoing: false, status: "received", created: created };
  }

  // checkFloodMovesHighlightOffAnEvictedRow reproduces a user's
  // highlight sitting on the one message loaded when a conversation is
  // opened, then a flood of Timeline.MAX_LOADED_ROWS further live
  // messages arriving - exactly enough to age that first message past
  // the capped timeline's own cap. applyMessage must move the highlight
  // to whatever is newest once its own row falls out, rather than leave
  // it pointing at a message that is no longer loaded. Returns false,
  // having already called Check.fail, on the first thing that does not
  // hold.
  function checkFloodMovesHighlightOffAnEvictedRow(): bool {
    conversationController.open({ id: "chat-1" });
    conversationController.applyMessage(root.message("m0", 0));
    conversationController.resetHighlight();
    if (conversationController.highlightedId !== "m0") {
      return Check.fail("setup: resetHighlight did not land on the only loaded message: got " + conversationController.highlightedId);
    }

    const cap = Timeline.MAX_LOADED_ROWS;
    for (let i = 1; i <= cap; i++) {
      conversationController.applyMessage(root.message("m" + i, i));
    }

    if (conversationController.highlightedId === "m0") {
      return Check.fail("a message flood should have aged the highlighted row out of the capped timeline and moved the highlight off it");
    }
    if (conversationController.highlightedId !== "m" + cap) {
      return Check.fail("the highlight should have moved to the newest loaded message the moment its own row fell out: got " + conversationController.highlightedId + ", want m" + cap);
    }

    return true;
  }

  // checkTypingMarksAnUnverifiedName confirms the header subtitle marks
  // a typing indicator's own name with "~" when the event reports it
  // unverified - a group member's own push name, say - and leaves a
  // verified one unmarked, so a stranger typing as "Mum" never reads
  // like a saved contact mid-conversation either. Returns false, having
  // already called Check.fail, on the first thing that does not hold.
  function checkTypingMarksAnUnverifiedName(): bool {
    conversationController.open({ id: "chat-1" });

    fakeService.event("typing", { conversationId: "chat-1", name: "Mum", verified: false, active: true });
    if (conversationController.subtitle !== "~Mum is typing…") {
      return Check.fail("unverified typing subtitle = " + conversationController.subtitle + ", want ~Mum is typing…");
    }

    fakeService.event("typing", { conversationId: "chat-1", name: "Alex", verified: true, active: true });
    if (conversationController.subtitle !== "Alex is typing…") {
      return Check.fail("verified typing subtitle = " + conversationController.subtitle + ", want unmarked Alex is typing…");
    }

    fakeService.event("typing", { conversationId: "chat-1", name: "Alex", verified: true, active: false });
    if (conversationController.subtitle === "Alex is typing…") {
      return Check.fail("typing stopped but the subtitle still shows it");
    }

    return true;
  }
}
