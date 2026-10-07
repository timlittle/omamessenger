// Checks MessageTimeline against a scripted service: a message that
// arrives as an event takes its place by time, older history at the bottom
// and new messages at the top; a page that arrives after older history
// already came in as events still puts its newer messages at the top; a
// page that repeats a message already shown does not show it twice; media()
// reads a loaded message's photo or video back; and photoNeighbor() steps
// between the photos in the loaded history, skipping messages without one.
import QtQuick
import Quickshell
import "ui/controllers"
import "ui/lib/Timeline.js" as Timeline
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

  // photoMessage is a minimal message carrying a photo.
  function photoMessage(id: string, created: int): var {
    return Object.assign(root.message(id, created), { media: { kind: "photo", width: 400, height: 300, thumb: "" } });
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

    timeline.upsert(root.photoMessage("m65", 65), false);
    timeline.upsert(root.photoMessage("m15", 15), false);
    if (root.ids() !== "m80,m70,m65,m60,m20,m15,m10") {
      Check.fail("photo messages were not placed by time: " + root.ids());
      return;
    }

    if (timeline.media("m70") !== null) {
      Check.fail("media() returned a photo for a message that has none");
      return;
    }
    if (!timeline.media("m65") || timeline.media("m65").kind !== "photo") {
      Check.fail("media() did not read back m65's photo");
      return;
    }

    if (timeline.photoNeighbor("m65", 1) !== "") {
      Check.fail("photoNeighbor found a newer photo that does not exist: " + timeline.photoNeighbor("m65", 1));
      return;
    }
    if (timeline.photoNeighbor("m65", -1) !== "m15") {
      Check.fail("photoNeighbor(-1) from m65 gave " + timeline.photoNeighbor("m65", -1) + ", want m15");
      return;
    }
    if (timeline.photoNeighbor("m15", 1) !== "m65") {
      Check.fail("photoNeighbor(1) from m15 gave " + timeline.photoNeighbor("m15", 1) + ", want m65");
      return;
    }
    if (timeline.photoNeighbor("unknown", 1) !== "") {
      Check.fail("photoNeighbor for an id that is not loaded should be \"\"");
      return;
    }

    // setReactions updates an existing row, not a freshly inserted one,
    // which is exactly the case a QML ListModel role holding an array of
    // objects loses silently; row() and reactions() keep it as JSON to
    // avoid that.
    timeline.setReactions("m60", [{ emoji: "👍", count: 1, mine: true }]);
    const reactions = Timeline.reactions(timeline.find("m60"));
    if (reactions.length !== 1 || reactions[0].emoji !== "👍") {
      Check.fail("setReactions did not stick: " + JSON.stringify(reactions));
      return;
    }

    console.log("PASS MessageTimeline");
    Qt.exit(0);
  }
}
