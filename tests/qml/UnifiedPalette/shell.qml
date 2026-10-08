// Checks the unified Ctrl+K palette against a scripted fake service (not
// the demo helper, so timing is deterministic apart from the message
// search's own debounce, which this test really waits out): opening the
// palette lists conversations at once; typing a query that matches no
// conversation title still turns up a message once the debounce and its
// round trip to the helper settle, in a "Messages" section with a
// sender, its conversation and a snippet; and accepting that row opens
// the conversation and highlights the exact message, not just whatever
// loads as newest. It also checks the "Toggle read receipts" command
// the palette offers, straight on the fake service's own toggle.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property int step: 0
  property int attempts: 0

  QtObject {
    id: service

    property var accounts: []
    property var services: []
    property var uiState: ({ railKey: "all", selectedId: "", query: "", drafts: {} })
    property var pendingAuth: null
    property string status: "ready"
    property string detail: ""
    property int unreadTotal: 0
    // readReceipts mirrors what a real Service.qml would track; the
    // command only needs to reach toggleReadReceipts, not the real
    // settings.apply round trip, which app-side tests already cover.
    property bool readReceipts: true

    signal event(string name, var data)

    function start(): void {}
    function installHelper(): void {}
    function quit(): void {}
    function toggleReadReceipts(): void { service.readReceipts = !service.readReceipts; }

    // request answers conversations.list and messages.list synchronously:
    // "Climbing Crew" is the only conversation, its title never matches
    // "ticket", but Priya's message in it does.
    function request(method: string, params: var, callback: var): void {
      if (method === "conversations.list") {
        if (params && params.query === "ticket") {
          callback(null, [
            { id: "climbing", title: "Climbing Crew", service: "whatsapp", lastActivity: 100, unread: 0,
              match: "got the ticket", matchMessageId: "m2", matchSender: "Priya" }
          ]);
          return;
        }
        callback(null, [{ id: "climbing", title: "Climbing Crew", service: "whatsapp", lastActivity: 100, unread: 2 }]);
        return;
      }

      if (method === "messages.list") {
        callback(null, { hasMore: false, messages: [
          { id: "m1", conversationId: "climbing", senderId: "s1", senderName: "Alex", text: "see you there", outgoing: false, status: "received", created: 1000 },
          { id: "m2", conversationId: "climbing", senderId: "s2", senderName: "Priya", text: "got the ticket", outgoing: false, status: "received", created: 2000 }
        ] });
        return;
      }

      callback(null, {});
    }
  }

  ListController {
    id: listController
    service: service
  }

  ConversationController {
    id: conversationController
    service: service
    listController: listController
  }

  WindowController {
    id: windowController
    service: service
    listController: listController
    conversationController: conversationController
  }

  property var steps: [
    root.waitForList,
    root.openPaletteAndType,
    root.waitForMessageSection,
    root.acceptMessage,
    root.waitForHighlight,
    root.toggleReceipts
  ]

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + reason);
    Qt.exit(1);
  }

  // waitForList holds until the conversation has loaded.
  function waitForList(): var {
    return listController.all.length === 1;
  }

  // openPaletteAndType opens the palette the way Ctrl+K does and types a
  // query that only a message matches.
  function openPaletteAndType(): var {
    windowController.run("palette.conversations");
    if (!windowController.paletteOpen)
      return root.fail("Ctrl+K did not open the palette");
    if (windowController.paletteItems.length !== 1 || windowController.paletteItems[0].label !== "Climbing Crew")
      return root.fail("the palette did not list the one known conversation before typing");

    windowController.setPaletteQuery("ticket");
    return true;
  }

  // waitForMessageSection polls for the debounced message search: the
  // "Conversations" section narrows to nothing ("ticket" matches no
  // title), and the "Messages" section gains Priya's message, naming
  // the conversation it is in and a snippet, once the round trip answers.
  function waitForMessageSection(): var {
    if (windowController.paletteResults.length !== 0) return false;
    if (windowController.paletteMessageResults.length !== 1) return false;

    const row = windowController.paletteMessageResults[0];
    if (row.sender !== "Priya" || row.conversationTitle !== "Climbing Crew" || row.snippet !== "got the ticket")
      return root.fail("message row = " + JSON.stringify(row));

    if (windowController.paletteItems.length !== 1 || windowController.paletteItems[0].section !== "message")
      return root.fail("paletteItems did not carry the message row: " + JSON.stringify(windowController.paletteItems));

    return true;
  }

  // acceptMessage runs Enter's action on the one row showing, the
  // message, since the empty "Conversations" section leaves it at
  // index 0.
  function acceptMessage(): var {
    windowController.run("palette.accept");
    if (windowController.paletteOpen) return root.fail("accepting a message row left the palette open");
    if (conversationController.activeId !== "climbing") return root.fail("accepting a message row did not open its conversation");
    return true;
  }

  // waitForHighlight polls until the conversation's initial page has
  // loaded and landed the highlight on the exact matched message, not
  // just whatever loads as newest (which would be m2 anyway here, so
  // this also confirms _pendingHighlightId, not a coincidence: see the
  // explicit id check).
  function waitForHighlight(): var {
    return conversationController.highlightedId === "m2";
  }

  // toggleReceipts runs the palette's incognito command straight on the
  // fake service, the same way WindowController.run always does.
  function toggleReceipts(): var {
    if (!service.readReceipts) return root.fail("service.readReceipts started false, cannot check the toggle");

    windowController.run("settings.toggleReadReceipts");
    if (service.readReceipts) return root.fail("settings.toggleReadReceipts did not turn read receipts off");

    windowController.run("settings.toggleReadReceipts");
    if (!service.readReceipts) return root.fail("settings.toggleReadReceipts did not turn read receipts back on");

    console.log("PASS UnifiedPalette");
    Qt.exit(0);
    return true;
  }

  Timer {
    running: true
    interval: 50
    repeat: true
    onTriggered: {
      const result = root.steps[root.step]();
      if (result === false) {
        root.attempts++;
        if (root.attempts > 100) root.fail("condition not met within 5 s");
        return;
      }
      if (result !== true) return; // fail() already called Qt.exit(1)

      root.attempts = 0;
      root.step++;
    }
  }
}
