// Checks ConversationView: the empty state with no conversation, loadOlder
// firing once scrolled to the oldest loaded message, and a submitted
// composer message reaching the send signal.
import QtQuick
import Quickshell
import "ui/components"
import "ui/lib/Timeline.js" as Timeline
import "Check.js" as Check

ShellRoot {
  id: root

  property var sent: []
  property int loadOlderCount: 0
  property int pollAttempts: 0

  // buildMessages returns count messages, newest first, one minute apart.
  function buildMessages(count) {
    const now = Date.now();
    const list = [];
    for (let i = 0; i < count; i++) {
      list.push({
        id: "m" + i, senderId: "s1", senderName: "Alex", text: "message " + i,
        outgoing: i % 2 === 0, status: "delivered", created: now - i * 60000
      });
    }
    return list;
  }

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 300
    visible: true

    ConversationView {
      id: view
      anchors.fill: parent
      onLoadOlder: root.loadOlderCount += 1
      onSend: text => root.sent.push(text)
    }
  }

  // pollTimer waits for the view's own content-height bindings to settle
  // after scrollToOldest(), since a 30-item ListView lays itself out over
  // several frames rather than in the one that calls it.
  Timer {
    id: pollTimer
    interval: 25
    repeat: true
    onTriggered: root.pollLoadOlder()
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives the view through each scenario and checks the outcome.
  function run(): void {
    if (!root.checkEmptyState()) return;
    root.startLoadOlderCheck();
  }

  // checkEmptyState verifies the placeholder shows when no chat is open.
  function checkEmptyState(): bool {
    const nodes = Check.texts(view);
    const empty = nodes.find(node => node.text === "Pick a chat · j/k to move · Enter to open");
    if (!empty || !empty.visible) return Check.fail("empty state not shown for a null conversation");
    return true;
  }

  // startLoadOlderCheck opens a 30-message conversation, scrolls to the
  // oldest loaded message and starts polling for loadOlder().
  function startLoadOlderCheck(): void {
    const now = Date.now();
    const messages = root.buildMessages(30);

    view.conversation = { id: "c1", accountId: "a1", service: "whatsapp", remoteId: "r1", kind: "direct", title: "Alex", members: 0, preview: "", muted: false, unread: 0, lastActivity: now };
    view.messages = messages;
    view.annotations = Timeline.annotate(messages, false, now);
    view.nowMs = now;

    root.loadOlderCount = 0;
    root.pollAttempts = 0;
    view.scrollToOldest();
    pollTimer.start();
  }

  // pollLoadOlder checks whether loadOlder() has fired yet, giving up
  // after four seconds.
  function pollLoadOlder(): void {
    root.pollAttempts += 1;
    if (root.loadOlderCount > 0) {
      pollTimer.stop();
      root.finishLoadOlderCheck();
      return;
    }
    if (root.pollAttempts < 160) return;

    pollTimer.stop();
    Check.fail("scrollToOldest() did not trigger loadOlder()");
  }

  // checkDraftRestore verifies a new draft still reaches the composer after
  // a message was sent, which clears the composer.
  function checkDraftRestore(): bool {
    view.draft = "first draft";
    view.composer.submit();
    view.draft = "second draft";
    if (view.composer.text !== "second draft")
      return Check.fail("composer shows \"" + view.composer.text + "\" after the draft changed, want \"second draft\"");
    return true;
  }

  // finishLoadOlderCheck runs the remaining checks once loadOlder() landed.
  function finishLoadOlderCheck(): void {
    if (!root.checkSend()) return;
    if (!root.checkDraftRestore()) return;

    console.log("PASS ConversationView");
    Qt.exit(0);
  }

  // checkSend verifies a submitted composer message is trimmed and reported.
  function checkSend(): bool {
    root.sent = [];
    view.composer.text = "  hello there  ";
    view.composer.submit();
    if (JSON.stringify(root.sent) !== '["hello there"]') return Check.fail("sent " + JSON.stringify(root.sent) + ", want [\"hello there\"]");
    return true;
  }
}
