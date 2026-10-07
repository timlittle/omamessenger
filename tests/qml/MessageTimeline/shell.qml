// Checks MessageTimeline against a scripted service: a message that
// arrives as an event takes its place by time, older history at the bottom
// and new messages at the top; a page that arrives after older history
// already came in as events still puts its newer messages at the top; and
// a page that repeats a message already shown does not show it twice.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  QtObject {
    id: service

    property var firstPage: null

    // request holds the first page until the test answers it, and answers
    // the older page at once.
    function request(method: string, params: var, callback: var): void {
      if (!params.before) {
        service.firstPage = callback;
        return;
      }
      callback(null, { hasMore: false, messages: [root.message("m10", 10), root.message("m20", 20)] });
    }
  }

  MessageTimeline {
    id: timeline
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // message is a minimal message with id and created time.
  function message(id: string, created: int): var {
    return { id: id, conversationId: "chat", senderId: "s", senderName: "S", text: id, outgoing: false, status: "received", created: created };
  }

  // ids lists the timeline's message ids, newest first.
  function ids(): string {
    const out = [];
    for (let i = 0; i < timeline.model.count; i++) out.push(timeline.model.get(i).id);
    return out.join(",");
  }

  // run loads a page, receives an old and a new message, then the older page.
  function run(): void {
    timeline.loadInitial(service, "chat", () => true, false);
    timeline.upsert(root.message("m20", 20), false);
    service.firstPage(null, { hasMore: true, messages: [root.message("m50", 50), root.message("m60", 60)] });
    timeline.upsert(root.message("m70", 70), false);
    if (root.ids() !== "m70,m60,m50,m20") {
      Check.fail("messages not placed by time: " + root.ids());
      return;
    }

    timeline.loadOlder(service, "chat", false);
    if (root.ids() !== "m70,m60,m50,m20,m10") {
      Check.fail("older page duplicated or misplaced a message: " + root.ids());
      return;
    }

    timeline.remove("m50", false);
    if (root.ids() !== "m70,m60,m20,m10") {
      Check.fail("remove did not drop the message: " + root.ids());
      return;
    }

    timeline.remove("m50", false);
    if (root.ids() !== "m70,m60,m20,m10") {
      Check.fail("removing an id twice changed the model: " + root.ids());
      return;
    }

    if (timeline.newestId() !== "m70") {
      Check.fail("newestId = " + timeline.newestId() + ", want m70");
      return;
    }

    const got = timeline.messageById("m60");
    if (!got || got.text !== "m60") {
      Check.fail("messageById did not return the loaded message");
      return;
    }
    if (timeline.messageById("missing") !== null) {
      Check.fail("messageById did not return null for an unloaded id");
      return;
    }

    timeline.upsert(Object.assign(root.message("m80", 80), { remoteId: "r80" }), false);
    if (timeline.localIdForRemote("r80") !== "m80") {
      Check.fail("localIdForRemote did not find the message by its remote id");
      return;
    }
    if (timeline.localIdForRemote("missing") !== "") {
      Check.fail("localIdForRemote did not return \"\" for an unknown remote id");
      return;
    }

    console.log("PASS MessageTimeline");
    Qt.exit(0);
  }
}
